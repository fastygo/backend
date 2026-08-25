package bbolt

import (
	"context"
	"slices"
	"strings"

	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	"github.com/fastygo/backend/internal/persist"
	bolt "go.etcd.io/bbolt"
)

type identityRepository struct {
	transaction *bolt.Tx
}

func (repository identityRepository) GetUser(_ context.Context, id string) (domainidentity.User, error) {
	value := repository.transaction.Bucket(usersBucket).Get([]byte(id))
	if value == nil {
		return domainidentity.User{}, ErrNotFound
	}
	return persist.DecodeUser(value)
}

func (repository identityRepository) GetUserByEmail(_ context.Context, email string) (domainidentity.User, error) {
	var resolved domainidentity.User
	err := repository.transaction.Bucket(usersBucket).ForEach(func(_, value []byte) error {
		user, err := persist.DecodeUser(value)
		if err != nil {
			return err
		}
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
		user, err := persist.DecodeUser(value)
		if err != nil {
			return err
		}
		users = append(users, user)
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
		persist.UserFromDomain(user),
		expectedVersion,
		func(encoded []byte) (uint64, error) {
			current, err := persist.DecodeUser(encoded)
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
	value := repository.transaction.Bucket(rolesBucket).Get([]byte(id))
	if value == nil {
		return domainidentity.Role{}, ErrNotFound
	}
	return persist.DecodeRole(value)
}

func (repository identityRepository) ListRoles(context.Context) ([]domainidentity.Role, error) {
	var roles []domainidentity.Role
	err := repository.transaction.Bucket(rolesBucket).ForEach(func(_, value []byte) error {
		role, err := persist.DecodeRole(value)
		if err != nil {
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
		persist.RoleFromDomain(role),
		expectedVersion,
		func(encoded []byte) (uint64, error) {
			current, err := persist.DecodeRole(encoded)
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
