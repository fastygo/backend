package persist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fastygo/backend/internal/domain/schema"
)

type Relation struct {
	Resource    string              `json:"resource"`
	Cardinality schema.Cardinality  `json:"cardinality"`
	OnDelete    schema.DeletePolicy `json:"on_delete"`
}

type Field struct {
	ID        string           `json:"id"`
	Type      schema.FieldType `json:"type"`
	Required  bool             `json:"required,omitempty"`
	Nullable  bool             `json:"nullable,omitempty"`
	ReadOnly  bool             `json:"read_only,omitempty"`
	Sensitive bool             `json:"sensitive,omitempty"`
	Localized bool             `json:"localized,omitempty"`
	Enum      []string         `json:"enum,omitempty"`
	Relation  *Relation        `json:"relation,omitempty"`
	Items     *Field           `json:"items,omitempty"`
}

type Resource struct {
	ID             string   `json:"id"`
	Collection     string   `json:"collection"`
	Fields         []Field  `json:"fields"`
	Form           []Field  `json:"form,omitempty"`
	Taxonomies     []string `json:"taxonomies,omitempty"`
	Public         bool     `json:"public,omitempty"`
	RESTVisible    bool     `json:"rest_visible,omitempty"`
	GraphQLVisible bool     `json:"graphql_visible,omitempty"`
}

type Manifest struct {
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Resources []Resource `json:"resources"`
}

func ManifestFromDomain(manifest schema.Manifest) Manifest {
	resources := make([]Resource, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, ResourceFromDomain(resource))
	}
	return Manifest{Name: manifest.Name, Version: manifest.Version, Resources: resources}
}

func ResourceFromDomain(resource schema.Resource) Resource {
	fields := make([]Field, 0, len(resource.Fields))
	for _, field := range resource.Fields {
		fields = append(fields, FieldFromDomain(field))
	}
	form := make([]Field, 0, len(resource.Form))
	for _, field := range resource.Form {
		form = append(form, FieldFromDomain(field))
	}
	return Resource{
		ID: resource.ID, Collection: resource.Collection, Fields: fields, Form: form,
		Taxonomies: resource.Taxonomies, Public: resource.Public,
		RESTVisible: resource.RESTVisible, GraphQLVisible: resource.GraphQLVisible,
	}
}

func FieldFromDomain(field schema.Field) Field {
	document := Field{
		ID: field.ID, Type: field.Type, Required: field.Required, Nullable: field.Nullable,
		ReadOnly: field.ReadOnly, Sensitive: field.Sensitive, Localized: field.Localized,
		Enum: field.Enum,
	}
	if field.Relation != nil {
		document.Relation = &Relation{
			Resource: field.Relation.Resource, Cardinality: field.Relation.Cardinality,
			OnDelete: field.Relation.OnDelete,
		}
	}
	if field.Items != nil {
		item := FieldFromDomain(*field.Items)
		document.Items = &item
	}
	return document
}

func (manifest Manifest) Domain() schema.Manifest {
	resources := make([]schema.Resource, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, resource.Domain())
	}
	return schema.Manifest{Name: manifest.Name, Version: manifest.Version, Resources: resources}
}

func (resource Resource) Domain() schema.Resource {
	fields := make([]schema.Field, 0, len(resource.Fields))
	for _, field := range resource.Fields {
		fields = append(fields, field.Domain())
	}
	form := make([]schema.Field, 0, len(resource.Form))
	for _, field := range resource.Form {
		form = append(form, field.Domain())
	}
	return schema.Resource{
		ID: resource.ID, Collection: resource.Collection, Fields: fields, Form: form,
		Taxonomies: resource.Taxonomies, Public: resource.Public,
		RESTVisible: resource.RESTVisible, GraphQLVisible: resource.GraphQLVisible,
	}
}

func (field Field) Domain() schema.Field {
	resolved := schema.Field{
		ID: field.ID, Type: field.Type, Required: field.Required, Nullable: field.Nullable,
		ReadOnly: field.ReadOnly, Sensitive: field.Sensitive, Localized: field.Localized,
		Enum: field.Enum,
	}
	if field.Relation != nil {
		resolved.Relation = &schema.Relation{
			Resource: field.Relation.Resource, Cardinality: field.Relation.Cardinality,
			OnDelete: field.Relation.OnDelete,
		}
	}
	if field.Items != nil {
		item := field.Items.Domain()
		resolved.Items = &item
	}
	return resolved
}

func EncodeManifest(manifest schema.Manifest) ([]byte, error) {
	encoded, err := json.Marshal(ManifestFromDomain(manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to encode schema manifest: %w", err)
	}
	return encoded, nil
}

func DecodeManifest(encoded []byte) (schema.Manifest, error) {
	var document Manifest
	if err := json.Unmarshal(encoded, &document); err != nil {
		return schema.Manifest{}, fmt.Errorf("failed to decode schema manifest: %w", err)
	}
	return document.Domain(), nil
}

func ManifestDigest(manifest schema.Manifest) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	encoded, err := EncodeManifest(manifest.Canonical())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
