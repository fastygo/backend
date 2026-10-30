package codeximport

import (
	"strings"
	"testing"
)

func TestParseForcedAllows(t *testing.T) {
	pol := ForcedPolicy{Tenant: "shop", Slot: "shop"}
	cases := map[string]Forced{
		"codex import --stdio --tenant=shop":                                      {Tenant: "shop"},
		"codex import --tenant=shop --stdio --dry-run":                            {Tenant: "shop", DryRun: true},
		"backend codex import --stdio --tenant=shop":                              {Tenant: "shop"},
		"/usr/local/bin/backend codex import --stdio --tenant=shop --dry-run":     {Tenant: "shop", DryRun: true},
		"incus exec shop -- backend codex import --stdio --tenant=shop --dry-run": {Tenant: "shop", DryRun: true},
		"  codex   import  --stdio  --tenant=shop ":                               {Tenant: "shop"},
	}
	for in, want := range cases {
		got, err := ParseForced(in, pol)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v; want %+v", in, got, err, want)
		}
	}
	got, _ := ParseForced("codex import --stdio --tenant=shop --dry-run", pol)
	if strings.Join(got.Args(), " ") != "--stdio --tenant=shop --dry-run" {
		t.Fatal(got.Args())
	}
}

func TestParseForcedRefuses(t *testing.T) {
	pol := ForcedPolicy{Tenant: "shop", Slot: "shop"}
	bad := []string{
		"",
		"sh",
		"codex export --stdio --tenant=shop",
		"codex import --tenant=shop",          // no --stdio
		"codex import --stdio",                // no tenant
		"codex import --stdio --tenant=press", // other tenant
		"codex import --stdio --tenant=shop --tenant=shop",  // twice
		"codex import --stdio --stdio --tenant=shop",        // twice
		"codex import --stdio --tenant=shop --listen=:8080", // unknown flag
		"codex import --stdio --tenant=shop extra",          // positional
		"codex import --stdio --tenant=shop; rm -rf /",      // metachar
		"codex import --stdio --tenant=shop && id",          // metachar
		"codex import --stdio --tenant=$(id)",               // metachar
		"codex import --stdio '--tenant=shop'",              // quotes
		"codex import --stdio --tenant=shop\nid",            // newline
		"codex import --stdio\t--tenant=shop",               // tab
		"incus exec press -- backend codex import --stdio --tenant=shop",
		"incus exec shop backend codex import --stdio --tenant=shop",
		"/tmp/backend codex import --stdio --tenant=shop",
		"codex import --stdio --tenant=shop " + strings.Repeat("x", 300),
	}
	for _, in := range bad {
		if _, err := ParseForced(in, pol); err == nil {
			t.Errorf("%q must be refused", in)
		}
	}
	if _, err := ParseForced("incus exec shop -- codex import --stdio --tenant=shop", ForcedPolicy{Tenant: "shop"}); err == nil {
		t.Error("incus prefix needs a pinned slot")
	}
	if _, err := ParseForced("codex import --stdio --tenant=shop", ForcedPolicy{}); err == nil {
		t.Error("policy without tenant must refuse")
	}
}
