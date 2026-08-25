package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/framework/pkg/core"
)

type resourceRecord struct {
	ID        domaincontent.ID   `json:"id"`
	Resource  domaincontent.Kind `json:"resource"`
	Version   uint64             `json:"version"`
	Values    map[string]any     `json:"values"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
}

type valuesDocument struct {
	ID      domaincontent.ID `json:"id,omitempty"`
	Version uint64           `json:"version,omitempty"`
	Values  map[string]any   `json:"values"`
}

func projectRecord(entry domaincontent.Entry) resourceRecord {
	values := make(map[string]any, len(entry.Metadata)+16)
	values["status"] = entry.Status
	values["visibility"] = entry.Visibility
	values["author_id"] = entry.AuthorID
	values["parent_id"] = entry.ParentID
	values["featured_media_id"] = entry.FeaturedMediaID
	values["template"] = entry.Template
	values["terms"] = entry.Terms
	values["slug"] = entry.Slug.Value("en", "")
	for locale, value := range entry.Slug {
		values["slug_"+locale] = value
	}
	for locale, value := range entry.Title {
		values["title_"+locale] = value
	}
	for locale, value := range entry.Content {
		values["content_"+locale] = value
	}
	for locale, value := range entry.Excerpt {
		values["excerpt_"+locale] = value
	}
	for key, value := range entry.Metadata {
		values[key] = value.Value
	}
	return resourceRecord{
		ID: entry.ID, Resource: entry.Kind, Version: entry.Version, Values: values,
		CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: entry.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func decodeEntryRequest(responseBody []byte) (domaincontent.Entry, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &probe); err != nil {
		return domaincontent.Entry{}, core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err)
	}
	if _, valuesEnvelope := probe["values"]; !valuesEnvelope {
		var entry domaincontent.Entry
		decoder := json.NewDecoder(strings.NewReader(string(responseBody)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entry); err != nil {
			return domaincontent.Entry{}, core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err)
		}
		return entry, nil
	}
	var document valuesDocument
	decoder := json.NewDecoder(strings.NewReader(string(responseBody)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return domaincontent.Entry{}, core.WrapDomainError(core.ErrorCodeValidation, "invalid values document", err)
	}
	entry := domaincontent.Entry{
		ID: document.ID, Version: document.Version,
		Slug: domaincontent.LocalizedText{}, Title: domaincontent.LocalizedText{},
		Content: domaincontent.LocalizedText{}, Excerpt: domaincontent.LocalizedText{},
		Metadata: map[string]domaincontent.MetadataValue{},
	}
	for key, value := range document.Values {
		switch {
		case key == "status":
			entry.Status = domaincontent.Status(asString(value))
		case key == "visibility":
			entry.Visibility = domaincontent.Visibility(asString(value))
		case key == "author_id":
			entry.AuthorID = asString(value)
		case key == "parent_id":
			entry.ParentID = domaincontent.ID(asString(value))
		case key == "featured_media_id":
			entry.FeaturedMediaID = asString(value)
		case key == "template":
			entry.Template = asString(value)
		case key == "slug":
			entry.Slug["en"] = asString(value)
		case localizedKey(key, "slug_"):
			entry.Slug[strings.TrimPrefix(key, "slug_")] = asString(value)
		case localizedKey(key, "title_"):
			entry.Title[strings.TrimPrefix(key, "title_")] = asString(value)
		case localizedKey(key, "content_"):
			entry.Content[strings.TrimPrefix(key, "content_")] = asString(value)
		case localizedKey(key, "excerpt_"):
			entry.Excerpt[strings.TrimPrefix(key, "excerpt_")] = asString(value)
		case key == "terms":
			encoded, _ := json.Marshal(value)
			if err := json.Unmarshal(encoded, &entry.Terms); err != nil {
				return domaincontent.Entry{}, fmt.Errorf("decode terms: %w", err)
			}
		default:
			entry.Metadata[key] = domaincontent.MetadataValue{Value: value}
		}
	}
	if len(entry.Title) == 0 {
		return domaincontent.Entry{}, errors.New("values document requires a localized title")
	}
	return entry, nil
}

func localizedKey(key, prefix string) bool {
	return strings.HasPrefix(key, prefix) && len(strings.TrimPrefix(key, prefix)) >= 2
}

func asString(value any) string {
	resolved, _ := value.(string)
	return resolved
}
