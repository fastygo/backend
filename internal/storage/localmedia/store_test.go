package localmedia

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestStoreRoundTripLimitAndPathSafety(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open media store: %v", err)
	}
	stored, err := store.Put(context.Background(), "media/file.txt", strings.NewReader("hello"), 5)
	if err != nil {
		t.Fatalf("put media: %v", err)
	}
	if stored.Size != 5 || len(stored.Checksum) != 64 {
		t.Fatalf("unexpected stored object: %#v", stored)
	}
	file, err := store.Open(context.Background(), stored.Key)
	if err != nil {
		t.Fatalf("open media: %v", err)
	}
	body, _ := io.ReadAll(file)
	_ = file.Close()
	if string(body) != "hello" {
		t.Fatalf("unexpected media body: %q", body)
	}
	if _, err := store.Put(context.Background(), "large.txt", strings.NewReader("too large"), 3); err == nil {
		t.Fatalf("oversized media was accepted")
	}
	if _, err := store.Open(context.Background(), "../secret"); err == nil {
		t.Fatalf("path traversal was accepted")
	}

	var backup bytes.Buffer
	if err := store.Export(context.Background(), &backup); err != nil {
		t.Fatalf("export media backup: %v", err)
	}
	restored, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open restore store: %v", err)
	}
	if err := restored.Restore(context.Background(), bytes.NewReader(backup.Bytes())); err != nil {
		t.Fatalf("restore media backup: %v", err)
	}
	restoredFile, err := restored.Open(context.Background(), "media/file.txt")
	if err != nil {
		t.Fatalf("open restored media: %v", err)
	}
	restoredBody, _ := io.ReadAll(restoredFile)
	_ = restoredFile.Close()
	if string(restoredBody) != "hello" {
		t.Fatalf("restored media mismatch: %q", restoredBody)
	}
}
