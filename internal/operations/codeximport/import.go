package codeximport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/backend/internal/delivery/rest"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/framework/pkg/core"
)

// Status values written on each per-record NDJSON line.
const (
	StatusCreated   = "created"
	StatusUpdated   = "updated"
	StatusUnchanged = "unchanged"
	StatusDeleted   = "deleted"
	StatusRejected  = "rejected"
)

const importerActor = "codex-import"

var blockedKinds = map[domaincontent.Kind]struct{}{
	domaincontent.KindMenu:    {},
	domaincontent.KindSetting: {},
	"media":                   {},
	"user":                    {},
}

type Options struct {
	DryRun bool
}

type Report struct {
	ID     string `json:"id,omitempty"`
	Locale string `json:"locale,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Slug   string `json:"slug,omitempty"`
	Status string `json:"status"`
	Digest string `json:"digest,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type Result struct {
	Reports  []Report
	Rejected int
}

type planned struct {
	report  Report
	op      string
	entry   domaincontent.Entry
	current *domaincontent.Entry
}

// Run reads NDJSON records from in, writes per-record reports then the tenant
// codex-manifest/v2 JCS object to out. It returns a non-nil error when any
// record is rejected. The batch is planned first; any reject skips writes.
func Run(
	ctx context.Context,
	service *application.Service,
	manifest schema.Manifest,
	in io.Reader,
	out io.Writer,
	opts Options,
) (Result, error) {
	if service == nil {
		return Result{}, fmt.Errorf("content service is required")
	}
	principal := authz.AdministratorRole().Principal(importerActor)
	plannedOps, err := plan(ctx, service, manifest, principal, in)
	if err != nil {
		return Result{}, err
	}
	result := Result{Reports: make([]Report, 0, len(plannedOps))}
	for _, item := range plannedOps {
		if item.report.Status == StatusRejected {
			result.Rejected++
		}
	}
	if result.Rejected == 0 && !opts.DryRun {
		if err := apply(ctx, service, principal, plannedOps); err != nil {
			return result, err
		}
	}
	for _, item := range plannedOps {
		result.Reports = append(result.Reports, item.report)
		if err := writeJSONLine(out, item.report); err != nil {
			return result, err
		}
	}
	if err := writeManifest(out, manifest); err != nil {
		return result, err
	}
	if result.Rejected > 0 {
		return result, fmt.Errorf("%d record(s) rejected", result.Rejected)
	}
	return result, nil
}

func plan(
	ctx context.Context,
	service *application.Service,
	manifest schema.Manifest,
	principal authz.Principal,
	in io.Reader,
) ([]planned, error) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var ops []planned
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		ops = append(ops, planLine(ctx, service, manifest, principal, lineNo, line))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return ops, nil
}

func planLine(
	ctx context.Context,
	service *application.Service,
	manifest schema.Manifest,
	principal authz.Principal,
	lineNo int,
	line []byte,
) planned {
	op, entry, providedDigest, err := decodeLine(line)
	if err != nil {
		return rejected("", "", "", fmt.Sprintf("line %d: %v", lineNo, err))
	}
	if op == "delete" {
		return planDelete(ctx, service, principal, entry)
	}
	return planUpsert(ctx, service, manifest, principal, entry, providedDigest)
}

func planUpsert(
	ctx context.Context,
	service *application.Service,
	manifest schema.Manifest,
	principal authz.Principal,
	entry domaincontent.Entry,
	providedDigest string,
) planned {
	if err := allowKind(manifest, entry.Kind); err != nil {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), err.Error())
	}
	digest, err := entryDigest(entry)
	if err != nil {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), "invalid codex-entry/v1: "+err.Error())
	}
	if providedDigest != "" && providedDigest != digest {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), "digest mismatch")
	}
	if err := entry.Validate(); err != nil {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), err.Error())
	}
	current, err := lookup(ctx, service, principal, entry)
	if err != nil {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), err.Error())
	}
	report := Report{
		ID: string(entry.ID), Locale: entry.Locale, Kind: string(entry.Kind),
		Slug: entry.Slug, Digest: digest, Status: StatusCreated,
	}
	item := planned{op: "upsert", entry: entry, report: report}
	if current == nil {
		return item
	}
	item.current = current
	item.report.ID = string(current.ID)
	existingDigest, err := entryDigest(*current)
	if err != nil {
		return rejected(string(current.ID), current.Locale, string(current.Kind), "existing record is not canonical: "+err.Error())
	}
	if existingDigest == digest {
		item.report.Status = StatusUnchanged
		item.report.Digest = existingDigest
		return item
	}
	item.report.Status = StatusUpdated
	return item
}

func planDelete(
	ctx context.Context,
	service *application.Service,
	principal authz.Principal,
	entry domaincontent.Entry,
) planned {
	current, err := lookup(ctx, service, principal, entry)
	if err != nil {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), err.Error())
	}
	if current == nil {
		return rejected(string(entry.ID), entry.Locale, string(entry.Kind), "record was not found")
	}
	if err := allowKind(schema.Manifest{}, current.Kind); err != nil {
		return rejected(string(current.ID), current.Locale, string(current.Kind), err.Error())
	}
	digest, _ := entryDigest(*current)
	return planned{
		op:      "delete",
		current: current,
		report: Report{
			ID: string(current.ID), Locale: current.Locale, Kind: string(current.Kind),
			Slug: current.Slug, Digest: digest, Status: StatusDeleted,
		},
	}
}

func apply(
	ctx context.Context,
	service *application.Service,
	principal authz.Principal,
	ops []planned,
) error {
	for _, item := range ops {
		switch item.report.Status {
		case StatusUnchanged, StatusRejected:
			continue
		case StatusCreated:
			if _, err := service.Create(ctx, principal, item.entry); err != nil {
				return err
			}
		case StatusUpdated:
			desired := item.entry
			desired.ID = item.current.ID
			if _, err := service.Update(ctx, principal, desired, item.current.Version, "codex import"); err != nil {
				return err
			}
		case StatusDeleted:
			if _, err := service.Transition(ctx, principal, item.current.ID, application.Transition{
				Kind:            item.current.Kind,
				Status:          domaincontent.StatusTrashed,
				ExpectedVersion: item.current.Version,
				Reason:          "codex import delete",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func lookup(
	ctx context.Context,
	service *application.Service,
	principal authz.Principal,
	entry domaincontent.Entry,
) (*domaincontent.Entry, error) {
	if strings.TrimSpace(string(entry.ID)) != "" {
		found, err := service.Get(ctx, principal, entry.ID)
		if err == nil {
			return &found, nil
		}
		if !isNotFound(err) {
			return nil, err
		}
	}
	if entry.Kind == "" || strings.TrimSpace(entry.Slug) == "" {
		return nil, nil
	}
	locale := entry.Locale
	if locale == "" {
		locale = domaincontent.DefaultLocale
	}
	found, err := service.GetBySlug(ctx, principal, entry.Kind, locale, entry.Slug)
	if err == nil {
		return &found, nil
	}
	if isNotFound(err) {
		return nil, nil
	}
	return nil, err
}

func allowKind(manifest schema.Manifest, kind domaincontent.Kind) error {
	if kind == "" {
		return fmt.Errorf("kind is required")
	}
	if _, blocked := blockedKinds[kind]; blocked {
		return fmt.Errorf("kind %q is not an importable long read", kind)
	}
	if manifest.Name == "" {
		return nil
	}
	for _, resource := range manifest.Resources {
		if resource.ID == string(kind) {
			return nil
		}
	}
	return fmt.Errorf("kind %q is not in the tenant manifest", kind)
}

func decodeLine(line []byte) (op string, entry domaincontent.Entry, digest string, err error) {
	var envelope struct {
		Op     string          `json:"op"`
		Digest string          `json:"digest"`
		ID     string          `json:"id"`
		Locale string          `json:"locale"`
		Kind   string          `json:"kind"`
		Slug   string          `json:"slug"`
		Entry  json.RawMessage `json:"entry"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return "", domaincontent.Entry{}, "", fmt.Errorf("invalid json")
	}
	op = strings.ToLower(strings.TrimSpace(envelope.Op))
	if op == "delete" {
		entry = domaincontent.Entry{
			ID: domaincontent.ID(envelope.ID), Locale: envelope.Locale,
			Kind: domaincontent.Kind(envelope.Kind), Slug: envelope.Slug,
		}
		if envelope.Entry != nil {
			if err := json.Unmarshal(envelope.Entry, &entry); err != nil {
				return "", domaincontent.Entry{}, "", fmt.Errorf("invalid entry")
			}
		}
		return op, entry, "", nil
	}
	raw := envelope.Entry
	if len(raw) == 0 {
		raw = line
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", domaincontent.Entry{}, "", fmt.Errorf("invalid codex-entry/v1")
	}
	if op == "" {
		op = "upsert"
	}
	if op != "upsert" {
		return "", domaincontent.Entry{}, "", fmt.Errorf("unsupported op %q", op)
	}
	return op, entry, strings.TrimSpace(envelope.Digest), nil
}

func entryDigest(entry domaincontent.Entry) (string, error) {
	projected := contentcompat.PublicCodex(entry)
	raw, err := json.Marshal(projected)
	if err != nil {
		return "", err
	}
	if _, err := canonical.Canonicalize(string(raw)); err != nil {
		return "", err
	}
	return canonical.Digest(string(raw))
}

func writeManifest(out io.Writer, manifest schema.Manifest) error {
	served := rest.ServedCodexManifest(manifest)
	raw, err := json.Marshal(served)
	if err != nil {
		return err
	}
	text, err := canonical.CanonicalizeManifest(string(raw))
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, text+"\n")
	return err
}

func writeJSONLine(out io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = out.Write(append(encoded, '\n'))
	return err
}

func rejected(id, locale, kind, reason string) planned {
	return planned{report: Report{ID: id, Locale: locale, Kind: kind, Status: StatusRejected, Reason: reason}}
}

func isNotFound(err error) bool {
	var domain core.DomainError
	return errors.As(err, &domain) && domain.Code == core.ErrorCodeNotFound
}
