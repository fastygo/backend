package main

import (
	"testing"

	"github.com/fastygo/backend/internal/domain/authz"
)

func TestResolveBuiltInAndCustomCapabilities(t *testing.T) {
	editor, err := resolveCapabilities("editor", "")
	if err != nil || len(editor) == 0 {
		t.Fatalf("resolve editor role: %v", err)
	}
	custom, err := resolveCapabilities("", "content.create,content.publish")
	if err != nil {
		t.Fatalf("resolve custom capabilities: %v", err)
	}
	if len(custom) != 2 || custom[1] != authz.CapabilityContentPublish {
		t.Fatalf("unexpected custom capabilities: %#v", custom)
	}
	if _, err := resolveCapabilities("", "unknown.capability"); err == nil {
		t.Fatalf("unknown custom capability was accepted")
	}
}
