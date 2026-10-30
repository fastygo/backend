package tenant

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// DefaultID is the implicit tenant when HEADLESS_TENANTS_PATH is unset.
const DefaultID = "default"

// Spec is one tenants.json object.
type Spec struct {
	ID        string         `json:"id"`
	Hosts     []string       `json:"hosts"`
	Storage   Storage        `json:"storage"`
	MediaRoot string         `json:"media_root"`
	Manifest  string         `json:"manifest"`
	Settings  map[string]any `json:"settings"`
}

// Storage locates one tenant's metadata. dsn is SQL (SQLite file, Postgres,
// MySQL); path is bbolt. driver is optional and inferred when empty.
type Storage struct {
	Driver string `json:"driver,omitempty"`
	DSN    string `json:"dsn,omitempty"`
	Path   string `json:"path,omitempty"`
}

// LoadFile reads tenants.json from path.
func LoadFile(path string) ([]Spec, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tenants file: %w", err)
	}
	defer file.Close()
	return Load(file)
}

// Load decodes a JSON array of Spec.
func Load(r io.Reader) ([]Spec, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var specs []Spec
	if err := decoder.Decode(&specs); err != nil {
		return nil, fmt.Errorf("tenants json: %w", err)
	}
	if err := Validate(specs, true); err != nil {
		return nil, err
	}
	return specs, nil
}

// Validate checks ids, hosts, and that storage/media never overlap.
func Validate(specs []Spec, multi bool) error {
	if len(specs) == 0 {
		return fmt.Errorf("tenants list is empty")
	}
	ids := map[string]struct{}{}
	hosts := map[string]string{}
	stores := map[string]string{}
	media := map[string]string{}
	for i, spec := range specs {
		id := strings.TrimSpace(spec.ID)
		if id == "" {
			return fmt.Errorf("tenants[%d]: id is required", i)
		}
		if strings.ContainsAny(id, " \t\n") {
			return fmt.Errorf("tenants[%d]: id must not contain whitespace", i)
		}
		if _, dup := ids[id]; dup {
			return fmt.Errorf("duplicate tenant id %q", id)
		}
		ids[id] = struct{}{}
		if multi {
			if len(spec.Hosts) == 0 {
				return fmt.Errorf("tenant %q: hosts is required", id)
			}
			if strings.TrimSpace(spec.Storage.DSN) == "" && strings.TrimSpace(spec.Storage.Path) == "" {
				return fmt.Errorf("tenant %q: storage.dsn or storage.path is required", id)
			}
			if strings.TrimSpace(spec.MediaRoot) == "" {
				return fmt.Errorf("tenant %q: media_root is required", id)
			}
		}
		for _, raw := range spec.Hosts {
			host := NormalizeHost(raw)
			if host == "" {
				return fmt.Errorf("tenant %q: host is empty", id)
			}
			if other, dup := hosts[host]; dup {
				return fmt.Errorf("host %q is claimed by tenants %q and %q", host, other, id)
			}
			hosts[host] = id
		}
		if loc := storageLocation(spec.Storage); loc != "" {
			if other, dup := stores[loc]; dup {
				return fmt.Errorf("storage %q is claimed by tenants %q and %q", loc, other, id)
			}
			stores[loc] = id
		}
		if root := strings.TrimSpace(spec.MediaRoot); root != "" {
			if other, dup := media[root]; dup {
				return fmt.Errorf("media_root %q is claimed by tenants %q and %q", root, other, id)
			}
			media[root] = id
		}
	}
	return nil
}

// NormalizeHost lowercases Host and strips a trailing port. IPv6 literals are
// accepted. X-Forwarded-Host is never read here.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return strings.ToLower(host)
}

// ResolveDriver returns the storage adapter name for this spec.
func (storage Storage) ResolveDriver(fallback string) string {
	if driver := strings.ToLower(strings.TrimSpace(storage.Driver)); driver != "" {
		return driver
	}
	dsn := strings.ToLower(strings.TrimSpace(storage.DSN))
	switch {
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		return "postgres"
	case strings.Contains(dsn, "@tcp("):
		return "mysql"
	case strings.TrimSpace(storage.DSN) != "":
		return "sqlite"
	case strings.TrimSpace(storage.Path) != "":
		return "bbolt"
	default:
		return strings.ToLower(strings.TrimSpace(fallback))
	}
}

func storageLocation(storage Storage) string {
	if dsn := strings.TrimSpace(storage.DSN); dsn != "" {
		return "dsn:" + dsn
	}
	if path := strings.TrimSpace(storage.Path); path != "" {
		return "path:" + path
	}
	return ""
}

// Language returns settings.language or settings.locale when set.
func (spec Spec) Language() string {
	if spec.Settings == nil {
		return ""
	}
	for _, key := range []string{"language", "locale"} {
		if text, ok := spec.Settings[key].(string); ok {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}
