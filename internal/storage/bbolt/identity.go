package bbolt

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	bolt "go.etcd.io/bbolt"
)

type identityRepository struct {
	transaction *bolt.Tx
}

func (repository identityRepository) GetUser(_ context.Context, id string) (domainidentity.User, error) {
	var record domainidentity.UserRecord
	err := getJSON(repository.transaction.Bucket(usersBucket), []byte(id), &record)
	return record.User(), err
}

func (repository identityRepository) GetUserByEmail(_ context.Context, email string) (domainidentity.User, error) {
	var resolved domainidentity.User
	err := repository.transaction.Bucket(usersBucket).ForEach(func(_, value []byte) error {
		var record domainidentity.UserRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		user := record.User()
		if strings.EqualFold(user.Email, strings.TrimSpace(email)) {
			resolved = user
		}
		return nil
	})
	if err != nil {
		return domainidentity.User{}, err
	}
	if resolved.ID == "" {
		return domainidentity.User{}, ErrNotFound
	}
	return resolved, nil
}

func (repository identityRepository) ListUsers(context.Context) ([]domainidentity.User, error) {
	var users []domainidentity.User
	err := repository.transaction.Bucket(usersBucket).ForEach(func(_, value []byte) error {
		var record domainidentity.UserRecord
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		users = append(users, record.User())
		return nil
	})
	slices.SortFunc(users, func(left, right domainidentity.User) int {
		return strings.Compare(left.Email, right.Email)
	})
	return users, err
}

func (repository identityRepository) SaveUser(
	_ context.Context,
	user domainidentity.User,
	expectedVersion uint64,
) error {
	return saveVersionedJSON(
		repository.transaction.Bucket(usersBucket),
		[]byte(user.ID),
		domainidentity.RecordFromUser(user),
		expectedVersion,
		func(encoded []byte) (uint64, error) {
			var current domainidentity.UserRecord
			err := json.Unmarshal(encoded, &current)
			return current.Version, err
		},
	)
}

func (repository identityRepository) DeleteUser(_ context.Context, id string, expectedVersion uint64) error {
	user, err := repository.GetUser(context.Background(), id)
	if err != nil {
		return err
	}
	if user.Version != expectedVersion {
		return ErrConflict
	}
	return repository.transaction.Bucket(usersBucket).Delete([]byte(id))
}

func (repository identityRepository) GetRole(_ context.Context, id string) (domainidentity.Role, error) {
	var role domainidentity.Role
	err := getJSON(repository.transaction.Bucket(rolesBucket), []byte(id), &role)
	return role, err
}

func (repository identityRepository) ListRoles(context.Context) ([]domainidentity.Role, error) {
	var roles []domainidentity.Role
	err := repository.transaction.Bucket(rolesBucket).ForEach(func(_, value []byte) error {
		var role domainidentity.Role
		if err := json.Unmarshal(value, &role); err != nil {
			return err
		}
		roles = append(roles, role)
		return nil
	})
	slices.SortFunc(roles, func(left, right domainidentity.Role) int {
		return strings.Compare(left.ID, right.ID)
	})
	return roles, err
}

func (repository identityRepository) SaveRole(
	_ context.Context,
	role domainidentity.Role,
	expectedVersion uint64,
) error {
	return saveVersionedJSON(
		repository.transaction.Bucket(rolesBucket),
		[]byte(role.ID),
		role,
		expectedVersion,
		func(encoded []byte) (uint64, error) {
			var current domainidentity.Role
			err := json.Unmarshal(encoded, &current)
			return current.Version, err
		},
	)
}

func (repository identityRepository) DeleteRole(_ context.Context, id string, expectedVersion uint64) error {
	role, err := repository.GetRole(context.Background(), id)
	if err != nil {
		return err
	}
	if role.Version != expectedVersion {
		return ErrConflict
	}
	return repository.transaction.Bucket(rolesBucket).Delete([]byte(id))
}

func getJSON(bucket *bolt.Bucket, key []byte, target any) error {
	value := bucket.Get(key)
	if value == nil {
		return ErrNotFound
	}
	return json.Unmarshal(value, target)
}

func saveVersionedJSON(
	bucket *bolt.Bucket,
	key []byte,
	value any,
	expectedVersion uint64,
	version func([]byte) (uint64, error),
) error {
	current := bucket.Get(key)
	if expectedVersion == 0 {
		if current != nil {
			return ErrConflict
		}
	} else {
		if current == nil {
			return ErrNotFound
		}
		resolved, err := version(current)
		if err != nil {
			return err
		}
		if resolved != expectedVersion {
			return ErrConflict
		}
	}
	return putJSON(bucket, key, value)
}
