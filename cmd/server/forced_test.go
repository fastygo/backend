package main

import (
	"strings"
	"testing"
)

func TestCodexForcedRefusesBeforeOpeningStorage(t *testing.T) {
	t.Setenv("SSH_ORIGINAL_COMMAND", "codex import --stdio --tenant=shop; id")
	err := runCodex([]string{"ssh-forced", "--tenant=shop"})
	if err == nil || !strings.Contains(err.Error(), "unexpected characters") {
		t.Fatalf("got %v", err)
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "codex import --stdio --tenant=press")
	if err := runCodex([]string{"ssh-forced", "--tenant=shop"}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("got %v", err)
	}
	if err := runCodex([]string{"ssh-forced", "--tenant=shop", "extra"}); err == nil {
		t.Fatal("positional args must be refused")
	}
}
