package schema

import (
	"errors"
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
	FieldObject     FieldType = "object"
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
	Fields    []Field
}

type Resource struct {
	ID             string
	Collection     string
	Fields         []Field
	Form           []Field
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

// CoreResources are the go-codex kinds every profile must keep.
func CoreResources() []Resource {
	documentForm := []Field{
		{ID: "title", Type: FieldString, Localized: true},
		{ID: "excerpt", Type: FieldText, Localized: true},
		{ID: "content", Type: FieldText, Localized: true},
	}
	return []Resource{
		{ID: "post", Collection: "posts", Public: true, RESTVisible: true, GraphQLVisible: true, Form: append([]Field(nil), documentForm...)},
		{ID: "page", Collection: "pages", Public: true, RESTVisible: true, GraphQLVisible: true, Form: append([]Field(nil), documentForm...)},
		{
			ID: "menu", Collection: "menus", Public: true, RESTVisible: true, GraphQLVisible: true,
			Fields: []Field{{ID: "items", Type: FieldJSON}},
			Form:   []Field{{ID: "items", Type: FieldJSON}},
		},
		{
			ID: "setting", Collection: "settings", Public: true, RESTVisible: true, GraphQLVisible: true,
			Fields: []Field{{ID: "value", Type: FieldJSON}},
			Form:   []Field{{ID: "value", Type: FieldJSON}},
		},
	}
}

// WithCoreResources adds reserved post/page/menu/setting kinds without replacing site resources.
func WithCoreResources(manifest Manifest) Manifest {
	seen := make(map[string]struct{}, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		seen[resource.ID] = struct{}{}
	}
	for _, resource := range CoreResources() {
		if _, exists := seen[resource.ID]; exists {
			continue
		}
		manifest.Resources = append(manifest.Resources, resource)
	}
	return manifest
}

var identifier = regexp.MustCompile(`^[a-z][a-zA-Z0-9_-]{0,62}$`)

// ReservedCodexCollections cannot be used as a CPT rest_base under /go-json/go/v2/.
func ReservedCodexCollections() []string {
	return []string{"media", "taxonomies", "search", "content-types", "types"}
}

// RegistersCodexCollection is WordPress show_in_rest: same posts controller on rest_base.
func (resource Resource) RegistersCodexCollection() bool {
	if !resource.RESTVisible {
		return false
	}
	switch resource.Collection {
	case "media", "taxonomies", "search", "content-types", "types", "menus", "settings":
		return false
	default:
		return true
	}
}

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
		if err := validateCodexCollection(resource); err != nil {
			return err
		}
		if _, exists := resources[resource.ID]; exists {
			return errors.New("resource identifier is duplicated")
		}
		if _, exists := collections[resource.Collection]; exists {
			return errors.New("resource collection is duplicated")
		}
		resources[resource.ID] = struct{}{}
		collections[resource.Collection] = struct{}{}
		if err := validateFields(resource.Fields, false); err != nil {
			return err
		}
		if err := validateFields(resource.Form, false); err != nil {
			return err
		}
		for _, taxonomy := range resource.Taxonomies {
			if !identifier.MatchString(taxonomy) {
				return errors.New("taxonomy identifier is invalid")
			}
		}
	}
	for _, resource := range manifest.Resources {
		for _, field := range append(append([]Field(nil), resource.Fields...), resource.Form...) {
			if err := validateRelationTarget(field, resources); err != nil {
				return err
			}
		}
	}
	return nil
}

func (manifest Manifest) Canonical() Manifest {
	canonical := manifest
	canonical.Resources = append([]Resource(nil), manifest.Resources...)
	slices.SortFunc(canonical.Resources, func(left, right Resource) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range canonical.Resources {
		canonical.Resources[index].Fields = cloneFields(canonical.Resources[index].Fields)
		slices.SortFunc(canonical.Resources[index].Fields, func(left, right Field) int {
			return strings.Compare(left.ID, right.ID)
		})
		canonical.Resources[index].Form = cloneFields(canonical.Resources[index].Form)
		slices.SortFunc(canonical.Resources[index].Form, func(left, right Field) int {
			return strings.Compare(left.ID, right.ID)
		})
		canonical.Resources[index].Taxonomies = append([]string(nil), canonical.Resources[index].Taxonomies...)
		slices.Sort(canonical.Resources[index].Taxonomies)
	}
	return canonical
}

func validateCodexCollection(resource Resource) error {
	for _, name := range ReservedCodexCollections() {
		if resource.Collection == name {
			return errors.New("resource collection is reserved")
		}
	}
	if resource.Collection == "menus" && resource.ID != "menu" {
		return errors.New("menus collection is reserved")
	}
	if resource.Collection == "settings" && resource.ID != "setting" {
		return errors.New("settings collection is reserved")
	}
	return nil
}

func cloneFields(fields []Field) []Field {
	out := make([]Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, cloneField(field))
	}
	return out
}

func cloneField(field Field) Field {
	cloned := field
	if field.Relation != nil {
		relation := *field.Relation
		cloned.Relation = &relation
	}
	if field.Items != nil {
		item := cloneField(*field.Items)
		cloned.Items = &item
	}
	cloned.Fields = cloneFields(field.Fields)
	slices.SortFunc(cloned.Fields, func(left, right Field) int {
		return strings.Compare(left.ID, right.ID)
	})
	cloned.Enum = append([]string(nil), field.Enum...)
	return cloned
}

func validateFields(fields []Field, nested bool) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !identifier.MatchString(field.ID) || (!nested && field.ID == "id") {
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
		if field.Type == FieldObject && len(field.Fields) == 0 {
			return errors.New("object field requires nested fields")
		}
		if field.Type != FieldCollection && field.Items != nil {
			return errors.New("item schema requires collection field type")
		}
		if field.Type != FieldObject && len(field.Fields) > 0 {
			return errors.New("nested fields require object field type")
		}
		if field.Items != nil {
			if err := validateFields([]Field{*field.Items}, true); err != nil {
				return err
			}
		}
		if err := validateFields(field.Fields, true); err != nil {
			return err
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
		if err := validateRelationTarget(*field.Items, resources); err != nil {
			return err
		}
	}
	for _, nested := range field.Fields {
		if err := validateRelationTarget(nested, resources); err != nil {
			return err
		}
	}
	return nil
}

func (fieldType FieldType) Valid() bool {
	switch fieldType {
	case FieldString, FieldText, FieldBoolean, FieldInteger, FieldNumber, FieldDecimal,
		FieldMoney, FieldDate, FieldDateTime, FieldURI, FieldUUID, FieldJSON, FieldEnum,
		FieldRelation, FieldCollection, FieldObject, FieldMedia:
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
