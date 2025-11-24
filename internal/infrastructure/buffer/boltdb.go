package buffer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Store wraps BoltDB to persist buffered operations while external services are unavailable.
type Store struct {
	db     *bolt.DB
	bucket []byte
}

// Open initializes the BoltDB file and ensures the bucket exists.
func Open(path string, bucket string) (*Store, error) {
	if bucket == "" {
		bucket = "buffer"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucket))
		return err
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{
		db:     db,
		bucket: []byte(bucket),
	}, nil
}

// Enqueue stores a buffer item using a priority-aware key.
func (s *Store) Enqueue(item Item) error {
	if s == nil || s.db == nil {
		return bolt.ErrDatabaseNotOpen
	}
	item.normalize()
	key := buildKey(item)
	item.bucketKey = []byte(key)

	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.bucket).Put(item.bucketKey, payload)
	})
}

// GetBatch returns up to limit items without removing them.
func (s *Store) GetBatch(limit int) ([]Item, error) {
	if s == nil || s.db == nil {
		return nil, bolt.ErrDatabaseNotOpen
	}
	if limit <= 0 {
		limit = 50
	}

	var items []Item
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(s.bucket).Cursor()
		for k, v := c.First(); k != nil && len(items) < limit; k, v = c.Next() {
			var item Item
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			item.bucketKey = append([]byte(nil), k...)
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

// Remove deletes the provided item from the buffer.
func (s *Store) Remove(item Item) error {
	if s == nil || s.db == nil {
		return bolt.ErrDatabaseNotOpen
	}
	if len(item.bucketKey) == 0 {
		return s.deleteByID(item.ID)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.bucket).Delete(item.bucketKey)
	})
}

// Requeue re-inserts an item after bumping its timestamp.
func (s *Store) Requeue(item Item) error {
	item.bucketKey = nil
	item.Timestamp = time.Now()
	return s.Enqueue(item)
}

// Size returns the number of buffered items.
func (s *Store) Size() (int, error) {
	if s == nil || s.db == nil {
		return 0, bolt.ErrDatabaseNotOpen
	}
	var count int
	err := s.db.View(func(tx *bolt.Tx) error {
		count = tx.Bucket(s.bucket).Stats().KeyN
		return nil
	})
	return count, err
}

// Cleanup removes items older than the provided timestamp.
func (s *Store) Cleanup(olderThan time.Time) error {
	if s == nil || s.db == nil {
		return bolt.ErrDatabaseNotOpen
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket(s.bucket).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item Item
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			if item.Timestamp.Before(olderThan) {
				if err := c.Delete(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Close closes the Bolt database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Stats exposes Bolt statistics for monitoring endpoints.
func (s *Store) Stats() bolt.Stats {
	if s == nil || s.db == nil {
		return bolt.Stats{}
	}
	return s.db.Stats()
}

func (s *Store) deleteByID(id string) error {
	if id == "" {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket(s.bucket).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item Item
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			if item.ID == id {
				return c.Delete()
			}
		}
		return nil
	})
}

func buildKey(item Item) string {
	return fmt.Sprintf("%d_%020d_%s", item.Priority, item.Timestamp.UnixNano(), item.ID)
}
