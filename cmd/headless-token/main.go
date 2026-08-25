package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/identity"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	subject := flag.String("subject", "", "stable service or user identifier")
	role := flag.String("role", "editor", "built-in role: editor or administrator")
	capabilities := flag.String("capabilities", "", "comma-separated capabilities overriding role")
	ttl := flag.Duration("ttl", 24*time.Hour, "token lifetime")
	issuer := flag.String("issuer", env("HEADLESS_TOKEN_ISSUER", "headless-backend"), "token issuer")
	flag.Parse()

	secret := strings.TrimSpace(os.Getenv("HEADLESS_TOKEN_SECRET"))
	manager, err := identity.NewTokenManager(secret, *issuer)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*subject) == "" {
		return errors.New("-subject is required")
	}
	selected, err := resolveCapabilities(*role, *capabilities)
	if err != nil {
		return err
	}
	token, err := manager.Issue(authz.NewPrincipal(strings.TrimSpace(*subject), selected...), *ttl)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, token)
	return err
}

func resolveCapabilities(role, raw string) ([]authz.Capability, error) {
	if strings.TrimSpace(raw) != "" {
		var capabilities []authz.Capability
		for _, value := range strings.Split(raw, ",") {
			capability := authz.Capability(strings.TrimSpace(value))
			if !capability.Valid() {
				return nil, fmt.Errorf("unknown capability %q", capability)
			}
			capabilities = append(capabilities, capability)
		}
		return capabilities, nil
	}
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "administrator":
		return authz.AdministratorRole().Capabilities, nil
	case "editor":
		return authz.EditorRole().Capabilities, nil
	default:
		return nil, errors.New("-role must be editor or administrator")
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
