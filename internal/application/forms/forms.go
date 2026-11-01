package forms

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/backend/internal/domain/schema"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/formset"
	"github.com/fastygo/framework/pkg/core"
)

// Record is the formset projection of a resource's editor form. It is built
// from the Codex record (schema.Resource.FormRecord) through formset.FromCodex,
// the single Codex → formset path; only editor UI hints are added here.
func Record(resource schema.Resource) formset.RecordType {
	record := formset.FromCodex(resource.FormRecord())
	record.Scope = formset.ScopeWorkspace
	for index := range record.Fields {
		applyUIHint(&record.Fields[index])
	}
	return record
}

func applyUIHint(field *formset.Field) {
	if field.UIHint != "" {
		return
	}
	switch field.Type {
	case formset.FieldRichText:
		field.UIHint = formset.UIHintTipTap
	case formset.FieldMarkdown:
		field.UIHint = formset.UIHintMarkdown
	case formset.FieldTextarea:
		field.UIHint = formset.UIHintTextarea
	}
}

func Schema(resource schema.Resource) (formset.JSONSchema, error) {
	record := Record(resource)
	if err := record.Validate(); err != nil {
		return nil, err
	}
	return formset.JSONSchemaFromRecord(record), nil
}

// LocaleDocumentsFromEntry reads per-locale JSON. During migration it still
// accepts metadata payload_<locale>. An entry with no locale rows uses other
// metadata as a single implicit document (caller must pass Bind locales).
func LocaleDocumentsFromEntry(entry domaincontent.Entry) map[string]map[string]any {
	contentcompat.Lift(&entry)
	documents := map[string]map[string]any{}
	for locale, document := range contentcompat.Documents(entry) {
		if document.Data != nil {
			documents[locale] = document.Data
		}
	}
	if len(documents) > 0 {
		return documents
	}
	if document := metadataDocument(entry); len(document) > 0 {
		documents["en"] = document
	}
	return documents
}

func Bind(resource schema.Resource, documents map[string]map[string]any, locales ...string) (formset.Form, error) {
	return formset.Bind(Record(resource), documents, locales...)
}

func BindEntry(resource schema.Resource, entry domaincontent.Entry) (formset.Form, error) {
	documents := LocaleDocumentsFromEntry(entry)
	locales := make([]string, 0, len(documents))
	for locale := range documents {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	return Bind(resource, documents, locales...)
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

func metadataDocument(entry domaincontent.Entry) map[string]any {
	document := map[string]any{}
	for key, value := range entry.Metadata {
		if strings.HasPrefix(key, "payload_") {
			continue
		}
		document[key] = value.Value
	}
	return document
}

func validatePayloadDocuments(entry domaincontent.Entry) error {
	for key, value := range entry.Metadata {
		if _, ok := strings.CutPrefix(key, "payload_"); !ok || value.Value == nil {
			continue
		}
		if asObject(value.Value) == nil {
			return core.NewDomainError(core.ErrorCodeValidation, key+" must be a JSON object")
		}
	}
	for locale, document := range contentcompat.Documents(entry) {
		if document.Data == nil {
			return core.NewDomainError(core.ErrorCodeValidation, locale+" locale data must be a JSON object")
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
