package registry

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Config is the storage subset a compiled driver needs.
type Config struct {
	DataSource string
	BboltPath  string
}

type Opener func(context.Context, Config) (any, error)

var (
	mu      sync.Mutex
	openers = map[string]Opener{}
)

// Register adds one compiled storage driver.
func Register(name string, open Opener) {
	mu.Lock()
	defer mu.Unlock()
	openers[strings.ToLower(strings.TrimSpace(name))] = open
}

// Open calls the driver selected for this build.
func Open(ctx context.Context, name string, config Config) (any, error) {
	key := canonical(name)
	mu.Lock()
	open := openers[key]
	mu.Unlock()
	if open == nil {
		return nil, fmt.Errorf("storage %q is not included in this build", name)
	}
	return open(ctx, config)
}

func canonical(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mariadb":
		return "mysql"
	case "postgresql":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
