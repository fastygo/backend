package registry

import (
	"context"
	"errors"
	"testing"
)

func TestRegisterOpenAndAliases(t *testing.T) {
	var seen Config
	Register(" MySQL ", func(_ context.Context, config Config) (any, error) {
		seen = config
		return "mysql-store", nil
	})
	Register("postgres", func(context.Context, Config) (any, error) { return nil, errors.New("boom") })
	store, err := Open(context.Background(), "MariaDB", Config{DataSource: "dsn"})
	if err != nil || store != "mysql-store" || seen.DataSource != "dsn" {
		t.Fatalf("mariadb alias: %v %v %#v", store, err, seen)
	}
	if _, err := Open(context.Background(), "postgresql", Config{}); err == nil || err.Error() != "boom" {
		t.Fatalf("postgresql alias must reach postgres opener: %v", err)
	}
	if _, err := Open(context.Background(), "oracle", Config{}); err == nil {
		t.Fatal("unknown driver must fail")
	}
	for input, want := range map[string]string{"MariaDB": "mysql", "\tPostgreSQL\n": "postgres", "BBOLT": "bbolt"} {
		if got := canonical(input); got != want {
			t.Errorf("canonical(%q) = %q want %q", input, got, want)
		}
	}
}
