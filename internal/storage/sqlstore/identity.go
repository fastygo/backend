package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	domainidentity "github.com/fastygo/backend/internal/domain/identity"
)

type identityRepository struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (repository identityRepository) GetUser(ctx context.Context, id string) (domainidentity.User, error) {
	return repository.getUser(ctx, "SELECT payload FROM identity_users WHERE id = ?", id)
}

func (repository identityRepository) GetUserByEmail(
	ctx context.Context,
	email string,
) (domainidentity.User, error) {
	return repository.getUser(
		ctx,
		"SELECT payload FROM identity_users WHERE email = ?",
		strings.ToLower(strings.TrimSpace(email)),
	)
}

func (repository identityRepository) getUser(
	ctx context.Context,
	query string,
	value any,
) (domainidentity.User, error) {
	var encoded []byte
	err := repository.transaction.QueryRowContext(ctx, bind(repository.dialect, query), value).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return domainidentity.User{}, ErrNotFound
	}
	if err != nil {
		return domainidentity.User{}, err
	}
	var record domainidentity.UserRecord
	err = json.Unmarshal(encoded, &record)
	return record.User(), err
}

func (repository identityRepository) ListUsers(ctx context.Context) ([]domainidentity.User, error) {
	rows, err := repository.transaction.QueryContext(ctx, "SELECT payload FROM identity_users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domainidentity.User
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var record domainidentity.UserRecord
		if err := json.Unmarshal(encoded, &record); err != nil {
			return nil, err
		}
		users = append(users, record.User())
	}
	slices.SortFunc(users, func(left, right domainidentity.User) int {
		return strings.Compare(left.Email, right.Email)
	})
	return users, rows.Err()
}

func (repository identityRepository) SaveUser(
	ctx context.Context,
	user domainidentity.User,
	expectedVersion uint64,
) error {
	encoded, err := json.Marshal(domainidentity.RecordFromUser(user))
	if err != nil {
		return err
	}
	if expectedVersion == 0 {
		_, err = repository.transaction.ExecContext(
			ctx,
			bind(repository.dialect,
				"INSERT INTO identity_users (id, email, version, payload) VALUES (?, ?, ?, ?)"),
			user.ID, user.Email, user.Version, encoded,
		)
		if err != nil {
			return ErrConflict
		}
		return nil
	}
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			`UPDATE identity_users SET email = ?, version = ?, payload = ?
			 WHERE id = ? AND version = ?`),
		user.Email, user.Version, encoded, user.ID, expectedVersion,
	)
	return versionedResult(result, err)
}

func (repository identityRepository) DeleteUser(
	ctx context.Context,
	id string,
	expectedVersion uint64,
) error {
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect, "DELETE FROM identity_users WHERE id = ? AND version = ?"),
		id,
		expectedVersion,
	)
	return versionedResult(result, err)
}

func (repository identityRepository) GetRole(ctx context.Context, id string) (domainidentity.Role, error) {
	var encoded []byte
	err := repository.transaction.QueryRowContext(
		ctx,
		bind(repository.dialect, "SELECT payload FROM identity_roles WHERE id = ?"),
		id,
	).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return domainidentity.Role{}, ErrNotFound
	}
	if err != nil {
		return domainidentity.Role{}, err
	}
	var role domainidentity.Role
	err = json.Unmarshal(encoded, &role)
	return role, err
}

func (repository identityRepository) ListRoles(ctx context.Context) ([]domainidentity.Role, error) {
	rows, err := repository.transaction.QueryContext(ctx, "SELECT payload FROM identity_roles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []domainidentity.Role
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var role domainidentity.Role
		if err := json.Unmarshal(encoded, &role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	slices.SortFunc(roles, func(left, right domainidentity.Role) int {
		return strings.Compare(left.ID, right.ID)
	})
	return roles, rows.Err()
}

func (repository identityRepository) SaveRole(
	ctx context.Context,
	role domainidentity.Role,
	expectedVersion uint64,
) error {
	encoded, err := json.Marshal(role)
	if err != nil {
		return err
	}
	if expectedVersion == 0 {
		_, err = repository.transaction.ExecContext(
			ctx,
			bind(repository.dialect,
				"INSERT INTO identity_roles (id, version, payload) VALUES (?, ?, ?)"),
			role.ID, role.Version, encoded,
		)
		if err != nil {
			return ErrConflict
		}
		return nil
	}
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			"UPDATE identity_roles SET version = ?, payload = ? WHERE id = ? AND version = ?"),
		role.Version, encoded, role.ID, expectedVersion,
	)
	return versionedResult(result, err)
}

func (repository identityRepository) DeleteRole(
	ctx context.Context,
	id string,
	expectedVersion uint64,
) error {
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect, "DELETE FROM identity_roles WHERE id = ? AND version = ?"),
		id,
		expectedVersion,
	)
	return versionedResult(result, err)
}
