package bbolt

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/fastygo/backend/internal/domain/taxonomy"
	bolt "go.etcd.io/bbolt"
)

type taxonomyRepository struct {
	transaction *bolt.Tx
}

func (repository taxonomyRepository) GetDefinition(_ context.Context, id string) (taxonomy.Definition, error) {
	value := repository.transaction.Bucket(taxonomiesBucket).Get([]byte(id))
	if value == nil {
		return taxonomy.Definition{}, ErrNotFound
	}
	var item taxonomy.Definition
	if err := json.Unmarshal(value, &item); err != nil {
		return taxonomy.Definition{}, err
	}
	return item, nil
}

func (repository taxonomyRepository) ListDefinitions(context.Context) ([]taxonomy.Definition, error) {
	items := make([]taxonomy.Definition, 0)
	err := repository.transaction.Bucket(taxonomiesBucket).ForEach(func(_, value []byte) error {
		var item taxonomy.Definition
		if err := json.Unmarshal(value, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	slices.SortFunc(items, func(left, right taxonomy.Definition) int {
		return strings.Compare(left.ID, right.ID)
	})
	return items, err
}

func (repository taxonomyRepository) SaveDefinition(
	_ context.Context,
	item taxonomy.Definition,
	expectedVersion uint64,
) error {
	bucket := repository.transaction.Bucket(taxonomiesBucket)
	key := []byte(item.ID)
	currentValue := bucket.Get(key)
	if expectedVersion == 0 {
		if currentValue != nil {
			return ErrConflict
		}
	} else {
		if currentValue == nil {
			return ErrNotFound
		}
		var current taxonomy.Definition
		if err := json.Unmarshal(currentValue, &current); err != nil {
			return err
		}
		if current.Version != expectedVersion {
			return ErrConflict
		}
	}
	return putJSON(bucket, key, item)
}

func (repository taxonomyRepository) DeleteDefinition(
	_ context.Context,
	id string,
	expectedVersion uint64,
) error {
	bucket := repository.transaction.Bucket(taxonomiesBucket)
	current, err := repository.GetDefinition(context.Background(), id)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	return bucket.Delete([]byte(id))
}

func (repository taxonomyRepository) GetTerm(_ context.Context, id taxonomy.ID) (taxonomy.Term, error) {
	value := repository.transaction.Bucket(termsBucket).Get([]byte(id))
	if value == nil {
		return taxonomy.Term{}, ErrNotFound
	}
	var item taxonomy.Term
	if err := json.Unmarshal(value, &item); err != nil {
		return taxonomy.Term{}, err
	}
	return item, nil
}

func (repository taxonomyRepository) ListTerms(_ context.Context, taxonomyID string) ([]taxonomy.Term, error) {
	items := make([]taxonomy.Term, 0)
	err := repository.transaction.Bucket(termsBucket).ForEach(func(_, value []byte) error {
		var item taxonomy.Term
		if err := json.Unmarshal(value, &item); err != nil {
			return err
		}
		if item.TaxonomyID == taxonomyID {
			items = append(items, item)
		}
		return nil
	})
	slices.SortFunc(items, func(left, right taxonomy.Term) int {
		return strings.Compare(string(left.ID), string(right.ID))
	})
	return items, err
}

func (repository taxonomyRepository) SaveTerm(
	_ context.Context,
	item taxonomy.Term,
	expectedVersion uint64,
) error {
	bucket := repository.transaction.Bucket(termsBucket)
	key := []byte(item.ID)
	currentValue := bucket.Get(key)
	if expectedVersion == 0 {
		if currentValue != nil {
			return ErrConflict
		}
	} else {
		if currentValue == nil {
			return ErrNotFound
		}
		var current taxonomy.Term
		if err := json.Unmarshal(currentValue, &current); err != nil {
			return err
		}
		if current.Version != expectedVersion {
			return ErrConflict
		}
	}
	return putJSON(bucket, key, item)
}

func (repository taxonomyRepository) DeleteTerm(
	_ context.Context,
	id taxonomy.ID,
	expectedVersion uint64,
) error {
	bucket := repository.transaction.Bucket(termsBucket)
	current, err := repository.GetTerm(context.Background(), id)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	return bucket.Delete([]byte(id))
}
