package rest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/codex/taxonomy"
)

const (
	// Local stand-ins for Codex v0.4.1 profile prefixes. Delete with the bump.
	settingsProfile      = "codex-settings/v1"
	settingsDigestPrefix = settingsProfile + ":sha256:"
)

const fallbackCanonicalErrorJCS = `{"error":{"code":"internal_error","message":"internal server error","status":500}}`

// encodeJCS is a local stand-in for canonical.JCS in Codex v0.4.1.
// Output is RFC 8785 with no trailing newline and no encoding/json HTML escapes.
func encodeJCS(value any) (string, error) {
	ready, err := jcsReady(value)
	if err != nil {
		return "", err
	}
	return canonical.Format(ready), nil
}

func writeCanonicalJCS(response http.ResponseWriter, status int, value any) {
	text, err := encodeJCS(value)
	if err != nil {
		writeCanonicalBytes(response, http.StatusInternalServerError, []byte(fallbackCanonicalErrorJCS), "")
		return
	}
	writeCanonicalBytes(response, status, []byte(text), "")
}

func jcsReady(value any) (any, error) {
	switch typed := value.(type) {
	case nil, bool, string, float64, json.Number, int, int32, int64, uint64:
		return typed, nil
	case uint:
		return uint64(typed), nil
	case float32:
		return float64(typed), nil
	case json.RawMessage:
		if len(typed) == 0 {
			return nil, nil
		}
		var parsed any
		if err := json.Unmarshal(typed, &parsed); err != nil {
			return nil, err
		}
		return jcsReady(parsed)
	case []byte:
		return jcsReady(json.RawMessage(typed))
	case []json.RawMessage:
		return jcsReadySlice(typed)
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			ready, err := jcsReady(item)
			if err != nil {
				return nil, err
			}
			items[index] = ready
		}
		return items, nil
	case []string:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = item
		}
		return items, nil
	case []domaincontent.Kind:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = string(item)
		}
		return items, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			ready, err := jcsReady(item)
			if err != nil {
				return nil, err
			}
			out[key] = ready
		}
		return out, nil
	case canonicalListDocument:
		return jcsReady(map[string]any{
			"data": typed.Data,
			"pagination": map[string]any{
				"page":        typed.Pagination.Page,
				"per_page":    typed.Pagination.PerPage,
				"total":       typed.Pagination.Total,
				"total_pages": typed.Pagination.TotalPages,
			},
		})
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		return jcsReady(json.RawMessage(raw))
	}
}

func jcsReadySlice[T any](items []T) (any, error) {
	out := make([]any, len(items))
	for index, item := range items {
		ready, err := jcsReady(item)
		if err != nil {
			return nil, err
		}
		out[index] = ready
	}
	return out, nil
}

func omitEmptyTopLevel(document map[string]any) map[string]any {
	for key, value := range document {
		if canonicalEmpty(value) {
			delete(document, key)
		}
	}
	return document
}

func canonicalEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func rfc3339Z(value time.Time) string {
	u := value.UTC().Truncate(time.Second)
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02dZ", u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second())
}

// canonicalizeTerm is a local stand-in for canonical.CanonicalizeTerm (codex-term/v1).
// The canonical form does not include count.
func canonicalizeTerm(item taxonomy.Term) ([]byte, error) {
	cloned := item.Clone()
	document := map[string]any{
		"created_at":  rfc3339Z(cloned.CreatedAt),
		"id":          string(cloned.ID),
		"locale":      cloned.Locale,
		"name":        cloned.Name,
		"slug":        cloned.Slug,
		"taxonomy_id": cloned.TaxonomyID,
		"updated_at":  rfc3339Z(cloned.UpdatedAt),
		"version":     cloned.Version,
	}
	if cloned.Description != "" {
		document["description"] = cloned.Description
	}
	if cloned.ParentID != "" {
		document["parent_id"] = string(cloned.ParentID)
	}
	if origin := originDocument(cloned.Origin); len(origin) > 0 {
		document["origin"] = origin
	}
	if metadata := publicMetadata(cloned.Metadata); len(metadata) > 0 {
		document["metadata"] = metadata
	}
	text, err := encodeJCS(document)
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

func canonicalizeTaxonomy(item taxonomy.Definition) ([]byte, error) {
	cloned := item.Canonical()
	kinds := make([]any, 0, len(cloned.AssignedToKinds))
	for _, kind := range cloned.AssignedToKinds {
		kinds = append(kinds, string(kind))
	}
	document := map[string]any{
		"assigned_to_kinds": kinds,
		"id":                cloned.ID,
		"label":             cloned.Label,
		"locale":            cloned.Locale,
		"mode":              string(cloned.Mode),
		"version":           cloned.Version,
	}
	if cloned.Public {
		document["public"] = true
	}
	text, err := encodeJCS(document)
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// settingsDocument is the SettingsV04 JSON shape. Isolated so Codex v0.4.1
// CanonicalizeSettings can replace this helper in one place.
func settingsDocument(settings domaincontent.Settings) map[string]any {
	document := map[string]any{
		"created_at": rfc3339Z(settings.CreatedAt),
		"id":         string(settings.ID),
		"language":   settings.Language,
		"locale":     settings.Locale,
		"title":      settings.Title,
		"updated_at": rfc3339Z(settings.UpdatedAt),
		"version":    settings.Version,
	}
	if settings.Description != "" {
		document["description"] = settings.Description
	}
	if settings.Timezone != "" {
		document["timezone"] = settings.Timezone
	}
	if settings.LogoMediaID != "" {
		document["logo_media_id"] = settings.LogoMediaID
	}
	if settings.IconMediaID != "" {
		document["icon_media_id"] = settings.IconMediaID
	}
	if settings.FrontPageID != "" {
		document["front_page_id"] = settings.FrontPageID
	}
	if settings.PostsPageID != "" {
		document["posts_page_id"] = settings.PostsPageID
	}
	if settings.PostsPerPage != nil {
		document["posts_per_page"] = *settings.PostsPerPage
	}
	if settings.CommentsOpen != nil {
		document["comments_open"] = *settings.CommentsOpen
	}
	if settings.Currency != "" {
		document["currency"] = settings.Currency
	}
	if origin := originDocument(settings.Origin); len(origin) > 0 {
		document["origin"] = origin
	}
	if metadata := publicMetadata(settings.Metadata); len(metadata) > 0 {
		document["metadata"] = metadata
	}
	return document
}

// canonicalizeSettings is a local stand-in for Codex v0.4.1 SettingsV04 canonicalization.
func canonicalizeSettings(settings domaincontent.Settings) ([]byte, string, error) {
	text, err := encodeJCS(settingsDocument(settings))
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256([]byte(text))
	return []byte(text), settingsDigestPrefix + hex.EncodeToString(sum[:]), nil
}

func settingsFromEntry(entry domaincontent.Entry) domaincontent.Settings {
	bag := metadataBag(entry.Metadata)
	settings := domaincontent.Settings{
		ID:          entry.ID,
		Locale:      entry.Locale,
		Title:       entry.Title,
		Description: firstNonEmpty(entry.Excerpt, entry.Content, metadataString(entry.Metadata, bag, "description")),
		Language:    firstNonEmpty(metadataString(entry.Metadata, bag, "language"), entry.Locale),
		Timezone:    metadataString(entry.Metadata, bag, "timezone"),
		LogoMediaID: metadataString(entry.Metadata, bag, "logo_media_id"),
		IconMediaID: metadataString(entry.Metadata, bag, "icon_media_id"),
		FrontPageID: metadataString(entry.Metadata, bag, "front_page_id"),
		PostsPageID: metadataString(entry.Metadata, bag, "posts_page_id"),
		Currency:    metadataString(entry.Metadata, bag, "currency"),
		Origin:      entry.Origin,
		Version:     entry.Version,
		CreatedAt:   entry.CreatedAt,
		UpdatedAt:   entry.UpdatedAt,
	}
	if value, ok := metadataInt(entry.Metadata, bag, "posts_per_page"); ok {
		settings.PostsPerPage = &value
	}
	if value, ok := metadataBool(entry.Metadata, bag, "comments_open"); ok {
		settings.CommentsOpen = &value
	}
	settings.Metadata = leftoverSettingsMetadata(entry.Metadata)
	return settings
}

var settingsFieldKeys = map[string]struct{}{
	"language": {}, "timezone": {}, "logo_media_id": {}, "icon_media_id": {},
	"front_page_id": {}, "posts_page_id": {}, "posts_per_page": {},
	"comments_open": {}, "currency": {}, "description": {},
}

func leftoverSettingsMetadata(metadata map[string]domaincontent.MetadataValue) map[string]domaincontent.MetadataValue {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]domaincontent.MetadataValue, len(metadata))
	for key, value := range metadata {
		if _, reserved := settingsFieldKeys[key]; reserved {
			continue
		}
		if value.Private {
			continue
		}
		out[key] = domaincontent.MetadataValue{Value: value.Value}
	}
	if bag, ok := out["value"]; ok {
		nested, isMap := bag.Value.(map[string]any)
		if isMap {
			trimmed := make(map[string]any, len(nested))
			for key, value := range nested {
				if _, reserved := settingsFieldKeys[key]; reserved {
					continue
				}
				trimmed[key] = value
			}
			if len(trimmed) == 0 {
				delete(out, "value")
			} else {
				out["value"] = domaincontent.MetadataValue{Value: trimmed}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metadataBag(metadata map[string]domaincontent.MetadataValue) map[string]any {
	value, ok := metadata["value"]
	if !ok {
		return nil
	}
	bag, _ := value.Value.(map[string]any)
	return bag
}

func metadataString(metadata map[string]domaincontent.MetadataValue, bag map[string]any, key string) string {
	if item, ok := metadata[key]; ok {
		if text, ok := item.Value.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	if bag != nil {
		if text, ok := bag[key].(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func metadataInt(metadata map[string]domaincontent.MetadataValue, bag map[string]any, key string) (int64, bool) {
	if item, ok := metadata[key]; ok {
		if value, ok := intFromJSON(item.Value); ok {
			return value, true
		}
	}
	if bag != nil {
		return intFromJSON(bag[key])
	}
	return 0, false
}

func metadataBool(metadata map[string]domaincontent.MetadataValue, bag map[string]any, key string) (bool, bool) {
	if item, ok := metadata[key]; ok {
		if value, ok := item.Value.(bool); ok {
			return value, true
		}
	}
	if bag != nil {
		value, ok := bag[key].(bool)
		return value, ok
	}
	return false, false
}

func intFromJSON(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			f, ferr := typed.Float64()
			if ferr != nil {
				return 0, false
			}
			return int64(f), true
		}
		return parsed, true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func originDocument(items []domaincontent.Origin) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{"id": item.ID, "system": item.System})
	}
	return out
}

func publicMetadata(metadata map[string]domaincontent.MetadataValue) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if value.Private {
			continue
		}
		out[key] = map[string]any{"value": value.Value}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// manifestDigestV2 is a local stand-in for schema.Manifest.DigestV2 in Codex v0.4.1:
// SHA-256 of JCS(full Canonical() manifest), empty optionals omitted only at top level.
func manifestDigestV2(manifest schema.Manifest) (string, error) {
	text, err := encodeJCS(omitEmptyTopLevel(fullCanonicalManifest(manifest)))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(text))
	return canonical.ManifestDigestV2Prefix + hex.EncodeToString(sum[:]), nil
}

func fullCanonicalManifest(manifest schema.Manifest) map[string]any {
	canonicalManifest := persist.ManifestFromDomain(manifest.Canonical())
	resources := make([]any, 0, len(canonicalManifest.Resources))
	for _, resource := range canonicalManifest.Resources {
		fields := resource.Fields
		if fields == nil {
			fields = []persist.Field{}
		}
		form := resource.Form
		if form == nil {
			form = []persist.Field{}
		}
		taxonomies := resource.Taxonomies
		if taxonomies == nil {
			taxonomies = []string{}
		}
		resources = append(resources, map[string]any{
			"collection":      resource.Collection,
			"fields":          fields,
			"form":            form,
			"graphql_visible": resource.GraphQLVisible,
			"id":              resource.ID,
			"public":          resource.Public,
			"rest_visible":    resource.RESTVisible,
			"taxonomies":      taxonomies,
		})
	}
	return map[string]any{
		"name":      canonicalManifest.Name,
		"resources": resources,
		"version":   canonicalManifest.Version,
	}
}

func sortSettings(items []domaincontent.Settings) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		return items[i].Locale < items[j].Locale
	})
}
