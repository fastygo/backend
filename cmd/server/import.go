package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/bootstrap"
	"github.com/fastygo/backend/internal/operations/codeximport"
)

func runCodex(args []string) error {
	if len(args) > 0 && args[0] == "ssh-forced" {
		return runCodexForced(args[1:])
	}
	if len(args) == 0 || args[0] != "import" {
		return fmt.Errorf("usage: backend codex import --stdio --tenant=<id> [--dry-run]\n       backend codex ssh-forced --tenant=<id> [--slot=<slot>]  (authorized_keys command=)")
	}
	return runCodexImport(args[1:])
}

// runCodexForced is the authorized_keys `command=` entry point. It reads
// $SSH_ORIGINAL_COMMAND, applies the strict allowlist pinned to one tenant,
// and only then runs the stdio import. See docs/runbooks/codex-import.md.
func runCodexForced(args []string) error {
	flags := flag.NewFlagSet("codex ssh-forced", flag.ContinueOnError)
	tenantID := flags.String("tenant", "", "tenant pinned to this key (required)")
	slot := flags.String("slot", "", "also accept the `incus exec <slot> --` prefix for this slot")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("codex ssh-forced takes no positional arguments")
	}
	inv, err := codeximport.ParseForced(os.Getenv("SSH_ORIGINAL_COMMAND"), codeximport.ForcedPolicy{Tenant: *tenantID, Slot: *slot})
	if err != nil {
		return err
	}
	return runCodexImport(inv.Args())
}

func runCodexImport(args []string) error {
	flags := flag.NewFlagSet("codex import", flag.ContinueOnError)
	stdio := flags.Bool("stdio", false, "read NDJSON records from stdin (required; no network listener)")
	tenantID := flags.String("tenant", "", "tenant id from tenants.json or default")
	dryRun := flags.Bool("dry-run", false, "print the plan only; write nothing")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*stdio {
		return fmt.Errorf("--stdio is required")
	}
	config, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	if config.Manifest.Name == "" {
		config.Manifest = bootstrap.DefaultManifest()
	}
	spec, err := bootstrap.LookupTenant(config, *tenantID)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	storage, manifest, err := bootstrap.OpenTenant(ctx, config, spec)
	if err != nil {
		return err
	}
	defer storage.Close()
	service, err := application.NewService(storage, nil, nil)
	if err != nil {
		return err
	}
	if err := service.SetManifest(manifest); err != nil {
		return err
	}
	_, err = codeximport.Run(ctx, service, manifest, os.Stdin, os.Stdout, codeximport.Options{DryRun: *dryRun})
	return err
}
