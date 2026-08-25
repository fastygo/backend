package localmedia

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (store *Store) Export(ctx context.Context, destination io.Writer) error {
	if destination == nil {
		return errors.New("media backup destination is required")
	}
	archive := tar.NewWriter(destination)
	err := filepath.WalkDir(store.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("media backup contains a non-regular file")
		}
		relative, err := filepath.Rel(store.root, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := copyContext(ctx, archive, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = archive.Close()
		return err
	}
	return archive.Close()
}

func (store *Store) Restore(ctx context.Context, source io.Reader) error {
	if source == nil {
		return errors.New("media backup source is required")
	}
	existing, err := os.ReadDir(store.root)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		return errors.New("media restore requires an empty store")
	}
	temporaryRoot, err := os.MkdirTemp(filepath.Dir(store.root), ".media-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryRoot)
	temporary := &Store{root: temporaryRoot}
	archive := tar.NewReader(source)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		//lint:ignore SA1019 TypeRegA is accepted for compatibility with legacy tar archives.
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return errors.New("media backup contains unsupported entry type")
		}
		key := strings.TrimSpace(header.Name)
		if header.Size < 0 {
			return errors.New("media backup entry size is invalid")
		}
		if _, err := temporary.Put(ctx, key, archive, header.Size); err != nil {
			return err
		}
	}
	if err := os.Remove(store.root); err != nil {
		return err
	}
	if err := os.Rename(temporaryRoot, store.root); err != nil {
		_ = os.MkdirAll(store.root, 0o750)
		return err
	}
	return nil
}
