package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type FieldType string
type Cardinality string
type DeletePolicy string

const (
	FieldString     FieldType = "string"
	FieldText       FieldType = "text"
	FieldBoolean    FieldType = "boolean"
	FieldInteger    FieldType = "integer"
	FieldNumber     FieldType = "number"
	FieldDecimal    FieldType = "decimal"
	FieldMoney      FieldType = "money"
	FieldDate       FieldType = "date"
	FieldDateTime   FieldType = "datetime"
	FieldURI        FieldType = "uri"
	FieldUUID       FieldType = "uuid"
	FieldJSON       FieldType = "json"
	FieldEnum       FieldType = "enum"
	FieldRelation   FieldType = "relation"
	FieldCollection FieldType = "collection"
	FieldMedia      FieldType = "media"
)

const (
	CardinalityOne  Cardinality = "one"
	CardinalityMany Cardinality = "many"
)

const (
	DeleteRestrict DeletePolicy = "restrict"
	DeleteNullify  DeletePolicy = "nullify"
	DeleteCascade  DeletePolicy = "cascade"
)

type Relation struct {
	Resource    string
	Cardinality Cardinality
	OnDelete    DeletePolicy
}

type Field struct {
	ID        string
	Type      FieldType
	Required  bool
	Nullable  bool
	ReadOnly  bool
	Sensitive bool
	Localized bool
	Enum      []string
	Relation  *Relation
	Items     *Field
}

type Resource struct {
	ID             string
	Collection     string
	Fields         []Field
	Taxonomies     []string
	Public         bool
	RESTVisible    bool
	GraphQLVisible bool
}

type Manifest struct {
	Name      string
	Version   string
	Resources []Resource
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

func (manifest Manifest) Validate() error {
	if strings.TrimSpace(manifest.Name) == "" || strings.TrimSpace(manifest.Version) == "" {
		return errors.New("manifest name and version are required")
	}
	resources := make(map[string]struct{}, len(manifest.Resources))
	collections := make(map[string]struct{}, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		if !identifier.MatchString(resource.ID) || !identifier.MatchString(resource.Collection) {
			return errors.New("resource identifier is invalid")
		}
		if _, exists := resources[resource.ID]; exists {
			return errors.New("resource identifier is duplicated")
		}
		if _, exists := collections[resource.Collection]; exists {
			return errors.New("resource collection is duplicated")
		}
		resources[resource.ID] = struct{}{}
		collections[resource.Collection] = struct{}{}
		if err := validateFields(resource.Fields); err != nil {
			return err
		}
		for _, taxonomy := range resource.Taxonomies {
			if !identifier.MatchString(taxonomy) {
				return errors.New("taxonomy identifier is invalid")
			}
		}
	}
	for _, resource := range manifest.Resources {
		for _, field := range resource.Fields {
			if err := validateRelationTarget(field, resources); err != nil {
				return err
			}
		}
	}
	return nil
}

func (manifest Manifest) Digest() (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	canonical := manifest
	canonical.Resources = append([]Resource(nil), manifest.Resources...)
	slices.SortFunc(canonical.Resources, func(left, right Resource) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range canonical.Resources {
		canonical.Resources[index].Fields = append([]Field(nil), canonical.Resources[index].Fields...)
		slices.SortFunc(canonical.Resources[index].Fields, func(left, right Field) int {
			return strings.Compare(left.ID, right.ID)
		})
		canonical.Resources[index].Taxonomies = append([]string(nil), canonical.Resources[index].Taxonomies...)
		slices.Sort(canonical.Resources[index].Taxonomies)
	}
	encoded, err := json.Marshal(digestManifest(canonical))
	if err != nil {
		return "", fmt.Errorf("failed to encode manifest digest: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateFields(fields []Field) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !identifier.MatchString(field.ID) || field.ID == "id" || field.ID == "version" {
			return errors.New("field identifier is invalid or reserved")
		}
		if _, exists := seen[field.ID]; exists {
			return errors.New("field identifier is duplicated")
		}
		seen[field.ID] = struct{}{}
		if !field.Type.Valid() {
			return errors.New("field type is invalid")
		}
		if field.Required && field.Nullable {
			return errors.New("required field cannot be nullable")
		}
		if field.Type == FieldEnum && len(field.Enum) == 0 {
			return errors.New("enum field requires values")
		}
		if field.Type == FieldRelation && field.Relation == nil {
			return errors.New("relation field requires relation metadata")
		}
		if field.Type == FieldRelation && field.Required &&
			field.Relation != nil && field.Relation.OnDelete == DeleteNullify {
			return errors.New("required relation cannot use nullify delete policy")
		}
		if field.Type != FieldRelation && field.Relation != nil {
			return errors.New("relation metadata requires relation field type")
		}
		if field.Type == FieldCollection && field.Items == nil {
			return errors.New("collection field requires item schema")
		}
		if field.Items != nil {
			if err := validateFields([]Field{*field.Items}); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRelationTarget(field Field, resources map[string]struct{}) error {
	if field.Relation != nil {
		if _, exists := resources[field.Relation.Resource]; !exists {
			return errors.New("relation target does not exist")
		}
		if !field.Relation.Cardinality.Valid() || !field.Relation.OnDelete.Valid() {
			return errors.New("relation policy is invalid")
		}
	}
	if field.Items != nil {
		return validateRelationTarget(*field.Items, resources)
	}
	return nil
}

func (fieldType FieldType) Valid() bool {
	switch fieldType {
	case FieldString, FieldText, FieldBoolean, FieldInteger, FieldNumber, FieldDecimal,
		FieldMoney, FieldDate, FieldDateTime, FieldURI, FieldUUID, FieldJSON, FieldEnum,
		FieldRelation, FieldCollection, FieldMedia:
		return true
	default:
		return false
	}
}

func (cardinality Cardinality) Valid() bool {
	return cardinality == CardinalityOne || cardinality == CardinalityMany
}

func (policy DeletePolicy) Valid() bool {
	return policy == DeleteRestrict || policy == DeleteNullify || policy == DeleteCascade
}

type digestRelation struct {
	Resource    string       `json:"resource"`
	Cardinality Cardinality  `json:"cardinality"`
	OnDelete    DeletePolicy `json:"on_delete"`
}

type digestField struct {
	ID        string          `json:"id"`
	Type      FieldType       `json:"type"`
	Required  bool            `json:"required,omitempty"`
	Nullable  bool            `json:"nullable,omitempty"`
	ReadOnly  bool            `json:"read_only,omitempty"`
	Sensitive bool            `json:"sensitive,omitempty"`
	Localized bool            `json:"localized,omitempty"`
	Enum      []string        `json:"enum,omitempty"`
	Relation  *digestRelation `json:"relation,omitempty"`
	Items     *digestField    `json:"items,omitempty"`
}

type digestResource struct {
	ID             string        `json:"id"`
	Collection     string        `json:"collection"`
	Fields         []digestField `json:"fields"`
	Taxonomies     []string      `json:"taxonomies,omitempty"`
	Public         bool          `json:"public,omitempty"`
	RESTVisible    bool          `json:"rest_visible,omitempty"`
	GraphQLVisible bool          `json:"graphql_visible,omitempty"`
}

type digestDocument struct {
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	Resources []digestResource `json:"resources"`
}

func digestManifest(manifest Manifest) digestDocument {
	resources := make([]digestResource, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		fields := make([]digestField, 0, len(resource.Fields))
		for _, field := range resource.Fields {
			fields = append(fields, digestFieldFrom(field))
		}
		resources = append(resources, digestResource{
			ID: resource.ID, Collection: resource.Collection, Fields: fields,
			Taxonomies: resource.Taxonomies, Public: resource.Public,
			RESTVisible: resource.RESTVisible, GraphQLVisible: resource.GraphQLVisible,
		})
	}
	return digestDocument{Name: manifest.Name, Version: manifest.Version, Resources: resources}
}

func digestFieldFrom(field Field) digestField {
	document := digestField{
		ID: field.ID, Type: field.Type, Required: field.Required, Nullable: field.Nullable,
		ReadOnly: field.ReadOnly, Sensitive: field.Sensitive, Localized: field.Localized,
		Enum: field.Enum,
	}
	if field.Relation != nil {
		document.Relation = &digestRelation{
			Resource: field.Relation.Resource, Cardinality: field.Relation.Cardinality,
			OnDelete: field.Relation.OnDelete,
		}
	}
	if field.Items != nil {
		item := digestFieldFrom(*field.Items)
		document.Items = &item
	}
	return document
}
