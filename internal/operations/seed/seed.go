package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/google/uuid"
)

const formatVersion = "fastygo.data.seed/v1"

var idempotencyNamespace = uuid.MustParse("33fb6db5-04e8-4a10-a650-fc22ddf7dc4e")

type Bundle struct {
	Version string   `json:"version"`
	Records []Record `json:"records"`
}

type Record struct {
	Resource       string         `json:"resource"`
	IdempotencyKey string         `json:"idempotency_key"`
	Values         map[string]any `json:"values"`
}

type Result struct {
	Created int
	Skipped int
}

func Apply(ctx context.Context, service *application.Service, principal authz.Principal, reader io.Reader) (Result, error) {
	if service == nil {
		return Result{}, fmt.Errorf("content service is required")
	}
	var bundle Bundle
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&bundle); err != nil {
		return Result{}, fmt.Errorf("failed to decode seed: %w", err)
	}
	if bundle.Version != formatVersion {
		return Result{}, fmt.Errorf("unsupported seed format %q", bundle.Version)
	}
	var result Result
	for _, record := range bundle.Records {
		created, err := applyRecord(ctx, service, principal, record)
		if err != nil {
			return result, err
		}
		if created {
			result.Created++
			continue
		}
		result.Skipped++
	}
	return result, nil
}

func applyRecord(
	ctx context.Context,
	service *application.Service,
	principal authz.Principal,
	record Record,
) (bool, error) {
	entry := entryFromRecord(record)
	if existing, err := service.GetBySlug(ctx, principal, entry.Kind, "en", entry.Slug.Value("en", "ru")); err == nil && existing.ID != "" {
		return false, nil
	}
	if _, err := service.Create(ctx, principal, entry); err != nil {
		if existing, lookupErr := service.Get(ctx, principal, entry.ID); lookupErr == nil && existing.ID == entry.ID {
			return false, nil
		}
		return false, fmt.Errorf("failed to seed %s %q: %w", record.Resource, record.IdempotencyKey, err)
	}
	return true, nil
}

func entryFromRecord(record Record) domaincontent.Entry {
	title := stringValue(record.Values["title"])
	slug := stringValue(record.Values["slug"])
	entry := domaincontent.Entry{
		ID: domaincontent.ID(uuid.NewSHA1(idempotencyNamespace, []byte(record.IdempotencyKey)).String()),
		Kind: domaincontent.Kind(record.Resource), Status: domaincontent.Status(stringValue(record.Values["status"])),
		Visibility: domaincontent.Visibility(stringValue(record.Values["visibility"])),
		Title:      domaincontent.LocalizedText{"en": title, "ru": title},
		Slug:       domaincontent.LocalizedText{"en": slug, "ru": slug},
		Content:    domaincontent.LocalizedText{},
		Excerpt:    domaincontent.LocalizedText{},
		Metadata:   map[string]domaincontent.MetadataValue{},
		Locales:    map[string]domaincontent.LocaleDocument{},
	}
	if raw, ok := record.Values["locales"]; ok {
		applyLocales(&entry, raw)
	}
	if raw, ok := record.Values["payload_ru"]; ok {
		applyPayloadLocale(&entry, "ru", raw)
	}
	if raw, ok := record.Values["payload_en"]; ok {
		applyPayloadLocale(&entry, "en", raw)
	}
	entry.LiftLocaleMetadata()
	if text := stringValue(record.Values["content"]); text != "" {
		entry.Content["en"] = text
		if entry.Content["ru"] == "" {
			entry.Content["ru"] = text
		}
	}
	if text := stringValue(record.Values["excerpt"]); text != "" {
		entry.Excerpt["en"] = text
		if entry.Excerpt["ru"] == "" {
			entry.Excerpt["ru"] = text
		}
	}
	return entry
}

func applyLocales(entry *domaincontent.Entry, raw any) {
	root, ok := raw.(map[string]any)
	if !ok {
		return
	}
	for locale, value := range root {
		data, _ := value.(map[string]any)
		if wrapped, ok := data["data"].(map[string]any); ok {
			data = wrapped
		}
		if data == nil {
			continue
		}
		applyPayloadLocale(entry, domaincontent.NormalizeLocale(locale), data)
	}
}

func applyPayloadLocale(entry *domaincontent.Entry, locale string, raw any) {
	document, ok := raw.(map[string]any)
	if !ok {
		return
	}
	locale = domaincontent.NormalizeLocale(locale)
	if entry.Locales == nil {
		entry.Locales = map[string]domaincontent.LocaleDocument{}
	}
	entry.Locales[locale] = domaincontent.LocaleDocument{Data: document, Status: entry.Status}
	if text := stringValue(document["title"]); text != "" {
		entry.Title[locale] = text
	}
	if text := stringValue(document["content"]); text != "" {
		entry.Content[locale] = text
	}
	if text := stringValue(document["excerpt"]); text != "" {
		entry.Excerpt[locale] = text
	}
}

func stringValue(value any) string {
	resolved, _ := value.(string)
	return strings.TrimSpace(resolved)
}
