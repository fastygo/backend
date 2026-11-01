package persist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fastygo/backend/internal/domain/schema"
)

// Field and Relation are persisted in Codex JSON form (codex/schema tags).
type (
	Field    = schema.Field
	Relation = schema.Relation
)

type Resource struct {
	ID             string     `json:"id"`
	Label          string     `json:"label,omitempty"`
	Collection     string     `json:"collection"`
	Fields         []Field    `json:"fields"`
	Form           []Field    `json:"form,omitempty"`
	Relations      []Relation `json:"relations,omitempty"`
	Taxonomies     []string   `json:"taxonomies,omitempty"`
	Public         bool       `json:"public,omitempty"`
	RESTVisible    bool       `json:"rest_visible,omitempty"`
	GraphQLVisible bool       `json:"graphql_visible,omitempty"`
}

// Type is the stable GET /go-json/go/v2/types document for one kind.
type Type struct {
	ID             string   `json:"id"`
	Collection     string   `json:"collection"`
	Public         bool     `json:"public"`
	RESTVisible    bool     `json:"rest_visible"`
	GraphQLVisible bool     `json:"graphql_visible"`
	Form           []Field  `json:"form"`
	Taxonomies     []string `json:"taxonomies"`
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

func TypeFromDomain(resource schema.Resource) Type {
	fields := resource.FormFields()
	if fields == nil {
		fields = []Field{}
	}
	taxonomies := resource.Taxonomies
	if taxonomies == nil {
		taxonomies = []string{}
	}
	return Type{
		ID: resource.ID, Collection: resource.Collection, Public: resource.Public,
		RESTVisible: resource.RESTVisible, GraphQLVisible: resource.GraphQLVisible,
		Form: fields, Taxonomies: taxonomies,
	}
}

func TypesFromManifest(manifest schema.Manifest) []Type {
	types := make([]Type, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		types = append(types, TypeFromDomain(resource))
	}
	return types
}

func ResourceFromDomain(resource schema.Resource) Resource {
	fields := append([]Field{}, resource.Fields...)
	return Resource{
		ID: resource.ID, Label: resource.Label, Collection: resource.Collection, Fields: fields,
		Form: append([]Field(nil), resource.Form...), Relations: append([]Relation(nil), resource.Relations...),
		Taxonomies: resource.Taxonomies, Public: resource.Public,
		RESTVisible: resource.RESTVisible, GraphQLVisible: resource.GraphQLVisible,
	}
}

func (manifest Manifest) Domain() schema.Manifest {
	resources := make([]schema.Resource, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, resource.Domain())
	}
	return schema.Manifest{Name: manifest.Name, Version: manifest.Version, Resources: resources}
}

func (resource Resource) Domain() schema.Resource {
	return schema.Resource{
		ID: resource.ID, Label: resource.Label, Collection: resource.Collection,
		Fields: append([]Field{}, resource.Fields...), Form: append([]Field{}, resource.Form...),
		Relations:  append([]Relation(nil), resource.Relations...),
		Taxonomies: resource.Taxonomies, Public: resource.Public,
		RESTVisible: resource.RESTVisible, GraphQLVisible: resource.GraphQLVisible,
	}
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
