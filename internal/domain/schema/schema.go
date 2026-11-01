// Package schema binds Codex records to this backend's delivery surface.
//
// The field, relation and rule vocabulary is github.com/fastygo/codex/schema
// (type aliases below). This package adds only deployment metadata that Codex
// does not model: the REST collection, visibility flags, a separate editor
// form, and the reserved core kinds. See docs/architecture/adr/0013.
package schema

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	codex "github.com/fastygo/codex/schema"
)

type (
	Field          = codex.Field
	FieldID        = codex.FieldID
	FieldType      = codex.FieldType
	Relation       = codex.Relation
	RelationID     = codex.RelationID
	Cardinality    = codex.RelationCardinality
	DeleteBehavior = codex.DeleteBehavior
	Option         = codex.Option
	ValidationRule = codex.ValidationRule
)

const (
	FieldText       = codex.FieldText
	FieldString     = codex.FieldString
	FieldTextarea   = codex.FieldTextarea
	FieldRichText   = codex.FieldRichText
	FieldMarkdown   = codex.FieldMarkdown
	FieldNumber     = codex.FieldNumber
	FieldBoolean    = codex.FieldBoolean
	FieldSelect     = codex.FieldSelect
	FieldDateTime   = codex.FieldDateTime
	FieldJSON       = codex.FieldJSON
	FieldObject     = codex.FieldObject
	FieldRelation   = codex.FieldRelation
	FieldCollection = codex.FieldCollection
	FieldComputed   = codex.FieldComputed
	FieldEncrypted  = codex.FieldEncrypted
)

const (
	OneToOne   = codex.RelationOneToOne
	OneToMany  = codex.RelationOneToMany
	ManyToMany = codex.RelationManyToMany
)

const (
	DeleteRestrict = codex.DeleteRestrict
	DeleteNullify  = codex.DeleteNullify
	DeleteCascade  = codex.DeleteCascade
)

// MediaTarget is the built-in relation target for media fields. Media assets
// live in the media store, not in a manifest resource.
const MediaTarget = "media"

// Resource is one Codex record bound to a delivery collection.
type Resource struct {
	ID         string
	Label      string
	Collection string
	// Fields are stored in Entry.Metadata and validated on write.
	Fields []Field
	// Form is the editor slot schema; empty means Fields (minus payload_*).
	Form []Field
	// Relations declare relation fields (Codex: relation ID == field ID).
	Relations      []Relation
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
		{ID: "title", Label: "Title", Type: FieldString, Localized: true},
		{ID: "excerpt", Label: "Excerpt", Type: FieldText, Localized: true},
		{ID: "content", Label: "Content", Type: FieldText, Localized: true},
	}
	return []Resource{
		{ID: "post", Collection: "posts", Public: true, RESTVisible: true, GraphQLVisible: true, Form: append([]Field(nil), documentForm...)},
		{ID: "page", Collection: "pages", Public: true, RESTVisible: true, GraphQLVisible: true, Form: append([]Field(nil), documentForm...)},
		{
			ID: "menu", Collection: "menus", Public: true, RESTVisible: true, GraphQLVisible: true,
			Fields: []Field{{ID: "items", Label: "Items", Type: FieldJSON}},
			Form:   []Field{{ID: "items", Label: "Items", Type: FieldJSON}},
		},
		{
			ID: "setting", Collection: "settings", Public: true, RESTVisible: true, GraphQLVisible: true,
			Fields: []Field{{ID: "value", Label: "Value", Type: FieldJSON}},
			Form:   []Field{{ID: "value", Label: "Value", Type: FieldJSON}},
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
	return !slices.Contains(ReservedCodexCollections(), resource.Collection)
}

// RelationFor returns the Codex relation declared for a top-level field.
func (resource Resource) RelationFor(id FieldID) (Relation, bool) {
	for _, relation := range resource.Relations {
		if relation.ID == RelationID(id) {
			return relation, true
		}
	}
	return Relation{}, false
}

// Record projects the resource into a Codex record type. Storage fields come
// first, then form-only fields; empty labels default to the field id.
func (resource Resource) Record() codex.RecordType {
	label := resource.Label
	if label == "" {
		label = HumanLabel(resource.ID)
	}
	record := codex.RecordType{
		ID:        codex.RecordTypeID(resource.ID),
		Label:     label,
		Scope:     codex.ScopeTenant,
		Relations: make([]Relation, 0, len(resource.Relations)),
	}
	if resource.Public {
		record.Visibility = "public"
	}
	seen := map[FieldID]struct{}{}
	for _, field := range append(append([]Field(nil), resource.Fields...), resource.FormFields()...) {
		if _, dup := seen[field.ID]; dup {
			continue
		}
		seen[field.ID] = struct{}{}
		record.Fields = append(record.Fields, withLabels(field))
	}
	for _, relation := range resource.Relations {
		if relation.Source == "" {
			relation.Source = record.ID
		}
		if relation.DeleteBehavior == "" {
			relation.DeleteBehavior = DeleteRestrict
		}
		record.Relations = append(record.Relations, relation)
	}
	if len(record.Relations) == 0 {
		record.Relations = nil
	}
	return record
}

// FormRecord is Record restricted to the editor form fields.
func (resource Resource) FormRecord() codex.RecordType {
	record := resource.Record()
	form := resource.FormFields()
	record.Fields = make([]Field, 0, len(form))
	for _, field := range form {
		record.Fields = append(record.Fields, withLabels(field))
	}
	return record
}

func withLabels(field Field) Field {
	if field.Label == "" {
		field.Label = HumanLabel(string(field.ID))
	}
	if field.Items != nil {
		item := withLabels(*field.Items)
		field.Items = &item
	}
	if field.Fields != nil {
		nested := make([]Field, len(field.Fields))
		for index, child := range field.Fields {
			nested[index] = withLabels(child)
		}
		field.Fields = nested
	}
	return field
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
		if err := validateRelations(resource, resources); err != nil {
			return err
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
		resource := &canonical.Resources[index]
		resource.Fields = cloneFields(resource.Fields)
		resource.Form = cloneFields(resource.Form)
		resource.Relations = append([]Relation(nil), resource.Relations...)
		slices.SortFunc(resource.Relations, func(left, right Relation) int {
			return strings.Compare(string(left.ID), string(right.ID))
		})
		resource.Taxonomies = append([]string(nil), resource.Taxonomies...)
		slices.Sort(resource.Taxonomies)
	}
	return canonical
}

func validateCodexCollection(resource Resource) error {
	if slices.Contains(ReservedCodexCollections(), resource.Collection) {
		return errors.New("resource collection is reserved")
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
	slices.SortFunc(out, func(left, right Field) int {
		return strings.Compare(string(left.ID), string(right.ID))
	})
	return out
}

func cloneField(field Field) Field {
	cloned := field
	if field.Items != nil {
		item := cloneField(*field.Items)
		cloned.Items = &item
	}
	if field.Fields != nil {
		cloned.Fields = cloneFields(field.Fields)
	}
	cloned.Options = append([]Option(nil), field.Options...)
	if field.Rules != nil {
		cloned.Rules = append([]ValidationRule(nil), field.Rules...)
	}
	return cloned
}

// KnownType reports whether t is a Codex v0.4 field type.
func KnownType(fieldType FieldType) bool {
	switch fieldType {
	case FieldText, FieldString, FieldTextarea, FieldRichText, FieldMarkdown, FieldNumber,
		FieldBoolean, FieldSelect, FieldDateTime, FieldJSON, FieldObject, FieldRelation,
		FieldCollection, FieldComputed, FieldEncrypted:
		return true
	default:
		return false
	}
}

// legacyTypes maps pre-Codex backend types to their Codex declaration.
var legacyTypes = map[FieldType]string{
	"integer": "number + " + codex.RuleInteger,
	"decimal": "number + " + codex.RuleDecimal,
	"money":   "number + " + codex.RuleMoney + " (integer minor units)",
	"date":    "datetime + " + codex.RuleDate,
	"uri":     "string + " + codex.RuleURI,
	"uuid":    "string + " + codex.RuleUUID,
	"enum":    "select + options",
	"media":   "relation + ui_hint media",
}

func validateFields(fields []Field, nested bool) error {
	seen := make(map[FieldID]struct{}, len(fields))
	for _, field := range fields {
		if !identifier.MatchString(string(field.ID)) || (!nested && field.ID == "id") {
			return errors.New("field identifier is invalid or reserved")
		}
		if _, exists := seen[field.ID]; exists {
			return errors.New("field identifier is duplicated")
		}
		seen[field.ID] = struct{}{}
		if !KnownType(field.Type) {
			if hint, legacy := legacyTypes[field.Type]; legacy {
				return fmt.Errorf("field %q type %q is not a Codex type; declare %s", field.ID, field.Type, hint)
			}
			return errors.New("field type is invalid")
		}
		if err := withLabels(field).Validate(); err != nil {
			return err
		}
		if err := codex.ValidateFieldProfile(field); err != nil {
			return err
		}
		if field.Type == FieldSelect && len(field.Options) == 0 {
			return errors.New("select field requires options")
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

func validateRelations(resource Resource, resources map[string]struct{}) error {
	declared := map[RelationID]Relation{}
	for _, relation := range resource.Relations {
		if _, dup := declared[relation.ID]; dup {
			return errors.New("relation identifier is duplicated")
		}
		declared[relation.ID] = relation
		if relation.Source != "" && string(relation.Source) != resource.ID {
			return errors.New("relation source must be the owning resource")
		}
		if _, exists := resources[string(relation.Target)]; !exists && relation.Target != MediaTarget {
			return errors.New("relation target does not exist")
		}
		switch relation.Cardinality {
		case OneToOne, OneToMany, ManyToMany:
		default:
			return errors.New("relation policy is invalid")
		}
		switch relation.DeleteBehavior {
		case "", DeleteRestrict, DeleteNullify, DeleteCascade:
		default:
			return errors.New("relation policy is invalid")
		}
	}
	used := map[RelationID]struct{}{}
	for _, field := range append(append([]Field(nil), resource.Fields...), resource.Form...) {
		if field.Type != FieldRelation {
			continue
		}
		relation, ok := declared[RelationID(field.ID)]
		if !ok {
			if IsMedia(field) {
				continue
			}
			return errors.New("relation field requires a declared relation")
		}
		used[relation.ID] = struct{}{}
		if IsMedia(field) && relation.Target != MediaTarget {
			return errors.New("media field must target media")
		}
		if field.Required && relation.DeleteBehavior == DeleteNullify {
			return errors.New("required relation cannot use nullify delete policy")
		}
	}
	for id := range declared {
		if _, ok := used[id]; !ok {
			return errors.New("relation is not bound to a relation field")
		}
	}
	return nil
}

// HumanLabel turns an identifier into a default label ("sale_price" → "Sale price").
func HumanLabel(identifier string) string {
	identifier = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(identifier, "_", " "), "-", " "))
	if identifier == "" {
		return ""
	}
	letters := []rune(identifier)
	letters[0] = unicode.ToUpper(letters[0])
	return string(letters)
}
