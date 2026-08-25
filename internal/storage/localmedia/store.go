package localmedia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	applicationmedia "github.com/fastygo/backend/internal/application/media"
)

type Store struct {
	root string
}

func Open(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("media root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve media root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("create media root: %w", err)
	}
	return &Store{root: absolute}, nil
}

func (store *Store) Put(
	ctx context.Context,
	key string,
	source io.Reader,
	maxBytes int64,
) (applicationmedia.StoredObject, error) {
	if source == nil || maxBytes <= 0 {
		return applicationmedia.StoredObject{}, errors.New("media source and positive limit are required")
	}
	target, err := store.resolve(key)
	if err != nil {
		return applicationmedia.StoredObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return applicationmedia.StoredObject{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return applicationmedia.StoredObject{}, err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	hash := sha256.New()
	written, err := copyContext(ctx, io.MultiWriter(temporary, hash), io.LimitReader(source, maxBytes+1))
	if err != nil {
		return applicationmedia.StoredObject{}, err
	}
	if written > maxBytes {
		return applicationmedia.StoredObject{}, errors.New("media file exceeds size limit")
	}
	if err := temporary.Sync(); err != nil {
		return applicationmedia.StoredObject{}, err
	}
	if err := temporary.Close(); err != nil {
		return applicationmedia.StoredObject{}, err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return applicationmedia.StoredObject{}, err
	}
	return applicationmedia.StoredObject{
		Key: key, Size: written, Checksum: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (store *Store) Open(_ context.Context, key string) (io.ReadCloser, error) {
	target, err := store.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.Open(target)
}

func (store *Store) Delete(_ context.Context, key string) error {
	target, err := store.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (store *Store) resolve(key string) (string, error) {
	key = filepath.Clean(filepath.FromSlash(strings.TrimSpace(key)))
	if key == "." || filepath.IsAbs(key) || key == ".." || strings.HasPrefix(key, ".."+string(filepath.Separator)) {
		return "", errors.New("media storage key is invalid")
	}
	target := filepath.Join(store.root, key)
	relative, err := filepath.Rel(store.root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("media storage key escapes root")
	}
	return target, nil
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			written, writeErr := destination.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != read {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
