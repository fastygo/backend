package tenant_test

import (
	"strings"
	"testing"

	"github.com/fastygo/backend/internal/tenant"
)

func TestNormalizeHostStripsPortAndLowercases(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Shop.Example.COM":      "shop.example.com",
		"shop.example.com:443":  "shop.example.com",
		"SHOP.EXAMPLE.COM:8080": "shop.example.com",
		"[::1]:8080":            "::1",
		"127.0.0.1:9000":        "127.0.0.1",
		"  press.example.com  ": "press.example.com",
	}
	for in, want := range cases {
		if got := tenant.NormalizeHost(in); got != want {
			t.Fatalf("NormalizeHost(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLoadRejectsDuplicateHostsAndUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := tenant.Load(strings.NewReader(`[
		{"id":"a","hosts":["shop.example.com"],"storage":{"path":"/tmp/a.db"},"media_root":"/tmp/a"},
		{"id":"b","hosts":["SHOP.example.com:443"],"storage":{"path":"/tmp/b.db"},"media_root":"/tmp/b"}
	]`))
	if err == nil || !strings.Contains(err.Error(), "shop.example.com") {
		t.Fatalf("duplicate host: %v", err)
	}
	_, err = tenant.Load(strings.NewReader(`[{"id":"a","hosts":["a.test"],"storage":{"path":"/tmp/a"},"media_root":"/tmp/a","extra":true}]`))
	if err == nil {
		t.Fatal("unknown field must fail")
	}
}

func TestStorageResolveDriver(t *testing.T) {
	t.Parallel()
	if got := (tenant.Storage{DSN: "/var/lib/shop.sqlite"}).ResolveDriver("bbolt"); got != "sqlite" {
		t.Fatalf("file dsn driver %q", got)
	}
	if got := (tenant.Storage{Path: "/var/lib/shop.db"}).ResolveDriver(""); got != "bbolt" {
		t.Fatalf("path driver %q", got)
	}
	if got := (tenant.Storage{DSN: "postgres://x"}).ResolveDriver(""); got != "postgres" {
		t.Fatalf("postgres driver %q", got)
	}
	if got := (tenant.Storage{DSN: "u:p@tcp(db:3306)/t"}).ResolveDriver(""); got != "mysql" {
		t.Fatalf("mysql driver %q", got)
	}
	if got := (tenant.Storage{Driver: "sqlite", Path: "/x"}).ResolveDriver("bbolt"); got != "sqlite" {
		t.Fatalf("explicit driver %q", got)
	}
}

func TestLoadHappyPath(t *testing.T) {
	t.Parallel()
	specs, err := tenant.Load(strings.NewReader(`[
		{
			"id":"shop",
			"hosts":["shop.example.com"],
			"storage":{"dsn":"/var/lib/shop.sqlite"},
			"media_root":"/var/lib/shop/media",
			"manifest":"/etc/shop.manifest.json",
			"settings":{"language":"en-US","title":"Latte Shop"}
		}
	]`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != "shop" || specs[0].Language() != "en-US" {
		t.Fatalf("spec: %#v", specs)
	}
	if specs[0].Storage.ResolveDriver("") != "sqlite" {
		t.Fatalf("driver %q", specs[0].Storage.ResolveDriver(""))
	}
}
