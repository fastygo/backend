package rest

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	codexschema "github.com/fastygo/codex/schema"
	"github.com/fastygo/codex/taxonomy"
)

const fallbackCanonicalErrorJCS = `{"error":{"code":"internal_error","message":"internal server error","status":500}}`

func writeCanonicalJCS(response http.ResponseWriter, status int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		writeCanonicalBytes(response, http.StatusInternalServerError, []byte(fallbackCanonicalErrorJCS), "")
		return
	}
	text, err := canonical.CanonicalJSON(string(raw))
	if err != nil {
		writeCanonicalBytes(response, http.StatusInternalServerError, []byte(fallbackCanonicalErrorJCS), "")
		return
	}
	writeCanonicalBytes(response, status, []byte(text), "")
}

func canonicalizeTerm(item taxonomy.Term) ([]byte, error) {
	cloned := item.Clone()
	cloned.CreatedAt = cloned.CreatedAt.UTC().Truncate(time.Second)
	cloned.UpdatedAt = cloned.UpdatedAt.UTC().Truncate(time.Second)
	raw, err := json.Marshal(cloned)
	if err != nil {
		return nil, err
	}
	text, err := canonical.CanonicalizeTerm(string(raw))
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

func canonicalizeTaxonomy(item taxonomy.Definition) ([]byte, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	text, err := canonical.CanonicalJSON(string(raw))
	if err != nil {
		return nil, err
	}
	return []byte(text), nil
}

func canonicalizeSettings(settings domaincontent.Settings) ([]byte, string, error) {
	settings.CreatedAt = settings.CreatedAt.UTC().Truncate(time.Second)
	settings.UpdatedAt = settings.UpdatedAt.UTC().Truncate(time.Second)
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, "", err
	}
	text, err := canonical.CanonicalizeSettings(string(raw))
	if err != nil {
		return nil, "", err
	}
	digest, err := canonical.SettingsDigest(string(raw))
	if err != nil {
		return nil, "", err
	}
	return []byte(text), digest, nil
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

func sortSettings(items []domaincontent.Settings) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		return items[i].Locale < items[j].Locale
	})
}

func manifestDigestV2(manifest schema.Manifest) (string, error) {
	return ServedCodexManifest(manifest).DigestV2()
}

// ServedCodexManifest projects the delivery schema into a Codex v0.4.1
// manifest. Resource order is the source order; Canonical() is not applied.
func ServedCodexManifest(manifest schema.Manifest) codexschema.Manifest {
	resources := make([]codexschema.Resource, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, codexResource(resource))
	}
	return codexschema.Manifest{
		Name:      manifest.Name,
		Version:   manifest.Version,
		Resources: resources,
	}
}

func codexResource(resource schema.Resource) codexschema.Resource {
	fields := resource.FormFields()
	if len(resource.Fields) > 0 {
		fields = resource.Fields
	}
	record := codexschema.RecordType{
		ID:     codexschema.RecordTypeID(resource.ID),
		Label:  resource.ID,
		Scope:  codexschema.ScopeTenant,
		Fields: make([]codexschema.Field, 0, len(fields)),
	}
	if resource.Public {
		record.Visibility = "public"
	}
	var relations []codexschema.Relation
	for _, field := range fields {
		record.Fields = append(record.Fields, codexField(field))
		if field.Type == schema.FieldRelation && field.Relation != nil {
			relations = append(relations, codexschema.Relation{
				ID:             codexschema.RelationID(field.ID),
				Source:         record.ID,
				Target:         codexschema.RecordTypeID(field.Relation.Resource),
				Cardinality:    codexCardinality(field.Relation.Cardinality),
				DeleteBehavior: codexschema.DeleteBehavior(field.Relation.OnDelete),
			})
		}
	}
	record.Relations = relations
	return codexschema.Resource{Record: record, Taxonomies: resource.Taxonomies}
}

func codexField(field schema.Field) codexschema.Field {
	out := codexschema.Field{
		ID:        codexschema.FieldID(field.ID),
		Label:     field.ID,
		Type:      codexschema.FieldType(field.Type),
		Required:  field.Required,
		Localized: field.Localized,
		Sensitive: field.Sensitive,
	}
	if field.ReadOnly {
		out.Rules = append(out.Rules, codexschema.ValidationRule{Name: codexschema.RuleReadOnly})
	}
	if field.Nullable {
		out.Rules = append(out.Rules, codexschema.ValidationRule{Name: codexschema.RuleNullable})
	}
	for _, value := range field.Enum {
		out.Options = append(out.Options, codexschema.Option{Value: value, Label: value})
	}
	if field.Items != nil {
		item := codexField(*field.Items)
		out.Items = &item
	}
	if len(field.Fields) > 0 {
		out.Fields = make([]codexschema.Field, 0, len(field.Fields))
		for _, nested := range field.Fields {
			out.Fields = append(out.Fields, codexField(nested))
		}
	}
	return out
}

func codexCardinality(cardinality schema.Cardinality) codexschema.RelationCardinality {
	if cardinality == schema.CardinalityMany {
		return codexschema.RelationManyToMany
	}
	return codexschema.RelationOneToOne
}
