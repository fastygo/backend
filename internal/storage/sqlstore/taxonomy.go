package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/backend/internal/persist"
)

type taxonomyRepository struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (repository taxonomyRepository) GetDefinition(
	ctx context.Context,
	id string,
) (taxonomy.Definition, error) {
	var encoded []byte
	err := repository.transaction.QueryRowContext(
		ctx,
		bind(repository.dialect, "SELECT payload FROM taxonomy_definitions WHERE id = ?"),
		id,
	).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return taxonomy.Definition{}, ErrNotFound
	}
	if err != nil {
		return taxonomy.Definition{}, err
	}
	return persist.DecodeDefinition(encoded)
}

func (repository taxonomyRepository) ListDefinitions(ctx context.Context) ([]taxonomy.Definition, error) {
	rows, err := repository.transaction.QueryContext(ctx, "SELECT payload FROM taxonomy_definitions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []taxonomy.Definition
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		item, err := persist.DecodeDefinition(encoded)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(left, right taxonomy.Definition) int {
		return strings.Compare(left.ID, right.ID)
	})
	return items, nil
}

func (repository taxonomyRepository) SaveDefinition(
	ctx context.Context,
	item taxonomy.Definition,
	expectedVersion uint64,
) error {
	encoded, err := persist.EncodeDefinition(item)
	if err != nil {
		return err
	}
	if expectedVersion == 0 {
		_, err = repository.transaction.ExecContext(
			ctx,
			bind(repository.dialect,
				"INSERT INTO taxonomy_definitions (id, version, payload) VALUES (?, ?, ?)"),
			item.ID,
			item.Version,
			encoded,
		)
		if err != nil {
			return ErrConflict
		}
		return nil
	}
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			"UPDATE taxonomy_definitions SET version = ?, payload = ? WHERE id = ? AND version = ?"),
		item.Version,
		encoded,
		item.ID,
		expectedVersion,
	)
	return versionedResult(result, err)
}

func (repository taxonomyRepository) DeleteDefinition(
	ctx context.Context,
	id string,
	expectedVersion uint64,
) error {
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect, "DELETE FROM taxonomy_definitions WHERE id = ? AND version = ?"),
		id,
		expectedVersion,
	)
	return versionedResult(result, err)
}

func (repository taxonomyRepository) GetTerm(ctx context.Context, id taxonomy.ID) (taxonomy.Term, error) {
	var encoded []byte
	err := repository.transaction.QueryRowContext(
		ctx,
		bind(repository.dialect, "SELECT payload FROM taxonomy_terms WHERE id = ?"),
		id,
	).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return taxonomy.Term{}, ErrNotFound
	}
	if err != nil {
		return taxonomy.Term{}, err
	}
	return persist.DecodeTerm(encoded)
}

func (repository taxonomyRepository) ListTerms(
	ctx context.Context,
	taxonomyID string,
) ([]taxonomy.Term, error) {
	rows, err := repository.transaction.QueryContext(
		ctx,
		bind(repository.dialect, "SELECT payload FROM taxonomy_terms WHERE taxonomy_id = ?"),
		taxonomyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []taxonomy.Term
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		item, err := persist.DecodeTerm(encoded)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(left, right taxonomy.Term) int {
		return strings.Compare(string(left.ID), string(right.ID))
	})
	return items, nil
}

func (repository taxonomyRepository) SaveTerm(
	ctx context.Context,
	item taxonomy.Term,
	expectedVersion uint64,
) error {
	encoded, err := persist.EncodeTerm(item)
	if err != nil {
		return err
	}
	if expectedVersion == 0 {
		_, err = repository.transaction.ExecContext(
			ctx,
			bind(repository.dialect,
				"INSERT INTO taxonomy_terms (id, taxonomy_id, version, payload) VALUES (?, ?, ?, ?)"),
			item.ID,
			item.TaxonomyID,
			item.Version,
			encoded,
		)
		if err != nil {
			return ErrConflict
		}
		return nil
	}
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			`UPDATE taxonomy_terms
			 SET taxonomy_id = ?, version = ?, payload = ?
			 WHERE id = ? AND version = ?`),
		item.TaxonomyID,
		item.Version,
		encoded,
		item.ID,
		expectedVersion,
	)
	return versionedResult(result, err)
}

func (repository taxonomyRepository) DeleteTerm(
	ctx context.Context,
	id taxonomy.ID,
	expectedVersion uint64,
) error {
	result, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect, "DELETE FROM taxonomy_terms WHERE id = ? AND version = ?"),
		id,
		expectedVersion,
	)
	return versionedResult(result, err)
}

func versionedResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}
