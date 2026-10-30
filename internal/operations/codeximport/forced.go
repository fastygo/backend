package codeximport

import (
	"fmt"
	"regexp"
	"strings"
)

// Forced is a parsed $SSH_ORIGINAL_COMMAND for the codex import key.
type Forced struct {
	Tenant string
	DryRun bool
}

// ForcedPolicy pins what one authorized key may run.
type ForcedPolicy struct {
	// Tenant is required: the key belongs to exactly one tenant.
	Tenant string
	// Slot, when set, also accepts the `incus exec <slot> --` prefix that
	// desktop clients send by default, but only for this slot.
	Slot string
}

const maxForcedLen = 256

var (
	forcedChars = regexp.MustCompile(`^[A-Za-z0-9 ._=/-]*$`)
	tenantID    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
)

// ParseForced applies a strict allowlist to $SSH_ORIGINAL_COMMAND. The only
// accepted shape is
//
//	[incus exec <slot> --] [backend|headless-backend|/usr/local/bin/backend] codex import --stdio --tenant=<id> [--dry-run]
//
// with flags in any order, each at most once. Anything else (shell
// metacharacters, quotes, extra arguments, other subcommands, another
// tenant) is refused before any storage is opened.
func ParseForced(original string, policy ForcedPolicy) (Forced, error) {
	if !tenantID.MatchString(policy.Tenant) {
		return Forced{}, fmt.Errorf("forced command: --tenant must pin a valid tenant id")
	}
	cmd := strings.TrimSpace(original)
	if cmd == "" {
		return Forced{}, fmt.Errorf("forced command: this key only runs `codex import --stdio --tenant=%s [--dry-run]`", policy.Tenant)
	}
	if len(cmd) > maxForcedLen {
		return Forced{}, fmt.Errorf("forced command: command too long")
	}
	if !forcedChars.MatchString(cmd) {
		return Forced{}, fmt.Errorf("forced command: unexpected characters")
	}
	words := strings.Fields(cmd)
	if len(words) >= 4 && words[0] == "incus" && words[1] == "exec" {
		if policy.Slot == "" || words[2] != policy.Slot || words[3] != "--" {
			return Forced{}, fmt.Errorf("forced command: incus prefix is not allowed for this key")
		}
		words = words[4:]
	}
	if len(words) > 0 {
		switch words[0] {
		case "backend", "headless-backend", "/usr/local/bin/backend", "/usr/local/bin/headless-backend":
			words = words[1:]
		}
	}
	if len(words) < 2 || words[0] != "codex" || words[1] != "import" {
		return Forced{}, fmt.Errorf("forced command: only `codex import` is allowed")
	}
	var out Forced
	seen := map[string]bool{}
	for _, w := range words[2:] {
		name := w
		if i := strings.IndexByte(w, '='); i >= 0 {
			name = w[:i]
		}
		if seen[name] {
			return Forced{}, fmt.Errorf("forced command: %s given twice", name)
		}
		seen[name] = true
		switch {
		case w == "--stdio":
		case w == "--dry-run":
			out.DryRun = true
		case strings.HasPrefix(w, "--tenant="):
			out.Tenant = strings.TrimPrefix(w, "--tenant=")
		default:
			return Forced{}, fmt.Errorf("forced command: argument %q is not allowed", w)
		}
	}
	if !seen["--stdio"] {
		return Forced{}, fmt.Errorf("forced command: --stdio is required")
	}
	if out.Tenant == "" {
		return Forced{}, fmt.Errorf("forced command: --tenant is required")
	}
	if out.Tenant != policy.Tenant {
		return Forced{}, fmt.Errorf("forced command: tenant %q is not the tenant pinned to this key", out.Tenant)
	}
	return out, nil
}

// Args returns the argv for runCodexImport.
func (f Forced) Args() []string {
	args := []string{"--stdio", "--tenant=" + f.Tenant}
	if f.DryRun {
		args = append(args, "--dry-run")
	}
	return args
}
