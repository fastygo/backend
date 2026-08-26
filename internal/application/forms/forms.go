package forms

import (
	"fmt"
	"strings"
	"unicode"

	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/formset"
	"github.com/fastygo/framework/pkg/core"
)

func Record(resource schema.Resource) formset.RecordType {
	fields := make([]formset.Field, 0, len(resource.FormFields()))
	relations := make([]formset.Relation, 0)
	for _, field := range resource.FormFields() {
		projected := projectField(field)
		fields = append(fields, projected)
		if field.Relation != nil {
			relations = append(relations, formset.Relation{
				ID:             formset.RelationID(field.ID),
				Label:          projected.Label,
				Source:         formset.RecordTypeID(resource.ID),
				Target:         formset.RecordTypeID(field.Relation.Resource),
				Cardinality:    cardinality(field.Relation.Cardinality),
				DeleteBehavior: deleteBehavior(field.Relation.OnDelete),
			})
		}
	}
	return formset.RecordType{
		ID:        formset.RecordTypeID(resource.ID),
		Label:     labelFor(resource.ID),
		Scope:     formset.ScopeWorkspace,
		Fields:    fields,
		Relations: relations,
	}
}

func Schema(resource schema.Resource) (formset.JSONSchema, error) {
	record := Record(resource)
	if err := record.Validate(); err != nil {
		return nil, err
	}
	return formset.JSONSchemaFromRecord(record), nil
}

func DocumentsFromEntry(entry domaincontent.Entry) formset.Documents {
	ru := asObject(metadataValue(entry, "payload_ru"))
	en := asObject(metadataValue(entry, "payload_en"))
	if ru == nil && en == nil {
		document := metadataDocument(entry)
		return formset.Documents{RU: document, EN: document}
	}
	return formset.Documents{RU: ru, EN: en}
}

func Bind(resource schema.Resource, documents formset.Documents) (formset.Form, error) {
	return formset.BindDocuments(Record(resource), documents)
}

func BindEntry(resource schema.Resource, entry domaincontent.Entry) (formset.Form, error) {
	return Bind(resource, DocumentsFromEntry(entry))
}

func ValidateEntry(resource schema.Resource, entry domaincontent.Entry) error {
	if err := validatePayloadDocuments(entry); err != nil {
		return err
	}
	if len(resource.Form) == 0 {
		return nil
	}
	form, err := BindEntry(resource, entry)
	if err != nil {
		return core.WrapDomainError(core.ErrorCodeValidation, "form schema is invalid", err)
	}
	if len(form.Issues) == 0 {
		return nil
	}
	issue := form.Issues[0]
	message := issue.Message
	if issue.Field != "" {
		message = fmt.Sprintf("%s: %s", issue.Field, issue.Message)
	}
	if issue.Locale != "" {
		message = issue.Locale + " " + message
	}
	return core.NewDomainError(core.ErrorCodeValidation, message)
}

func projectField(field schema.Field) formset.Field {
	projected := formset.Field{
		ID:        formset.FieldID(field.ID),
		Label:     labelFor(field.ID),
		Type:      projectType(field),
		Required:  field.Required,
		Localized: field.Localized,
		Sensitive: field.Sensitive,
	}
	for _, value := range field.Enum {
		projected.Options = append(projected.Options, formset.Option{Value: value, Label: labelFor(value)})
	}
	if field.Items != nil {
		item := projectField(*field.Items)
		projected.Items = &item
	}
	return projected
}

func projectType(field schema.Field) formset.FieldType {
	switch field.Type {
	case schema.FieldText:
		return formset.FieldTextarea
	case schema.FieldBoolean:
		return formset.FieldBoolean
	case schema.FieldInteger, schema.FieldNumber, schema.FieldDecimal, schema.FieldMoney:
		return formset.FieldNumber
	case schema.FieldDate, schema.FieldDateTime:
		return formset.FieldDateTime
	case schema.FieldEnum:
		return formset.FieldSelect
	case schema.FieldRelation:
		return formset.FieldRelation
	case schema.FieldCollection:
		return formset.FieldCollection
	case schema.FieldJSON:
		return formset.FieldJSON
	default:
		return formset.FieldText
	}
}

func cardinality(value schema.Cardinality) formset.RelationCardinality {
	if value == schema.CardinalityMany {
		return formset.RelationOneToMany
	}
	return formset.RelationOneToOne
}

func deleteBehavior(policy schema.DeletePolicy) formset.DeleteBehavior {
	switch policy {
	case schema.DeleteCascade:
		return formset.DeleteCascade
	case schema.DeleteNullify:
		return formset.DeleteNullify
	default:
		return formset.DeleteRestrict
	}
}

func metadataValue(entry domaincontent.Entry, key string) any {
	if entry.Metadata == nil {
		return nil
	}
	value, exists := entry.Metadata[key]
	if !exists {
		return nil
	}
	return value.Value
}

func metadataDocument(entry domaincontent.Entry) map[string]any {
	document := map[string]any{}
	for key, value := range entry.Metadata {
		if key == "payload_ru" || key == "payload_en" {
			continue
		}
		document[key] = value.Value
	}
	return document
}

func validatePayloadDocuments(entry domaincontent.Entry) error {
	for _, key := range []string{"payload_ru", "payload_en"} {
		value, exists := entry.Metadata[key]
		if !exists || value.Value == nil {
			continue
		}
		if asObject(value.Value) == nil {
			return core.NewDomainError(core.ErrorCodeValidation, key+" must be a JSON object")
		}
	}
	return nil
}

func asObject(value any) map[string]any {
	switch document := value.(type) {
	case map[string]any:
		return document
	case nil:
		return nil
	default:
		return nil
	}
}

func labelFor(identifier string) string {
	identifier = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(identifier, "_", " "), "-", " "))
	if identifier == "" {
		return ""
	}
	letters := []rune(identifier)
	letters[0] = unicode.ToUpper(letters[0])
	return string(letters)
}
