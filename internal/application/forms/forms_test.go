package forms

import (
	"testing"

	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/schema"
)

func TestBindKeepsUnknownPayloadKeys(t *testing.T) {
	t.Parallel()
	resource := schema.Resource{
		ID: "product", Collection: "products",
		Fields: []schema.Field{
			{ID: "payload_ru", Type: schema.FieldJSON},
			{ID: "payload_en", Type: schema.FieldJSON},
		},
		Form: []schema.Field{
			{ID: "title", Type: schema.FieldString, Required: true, Localized: true},
			{ID: "price", Type: schema.FieldMoney},
		},
	}
	form, err := BindEntry(resource, domaincontent.Entry{
		Kind: "product",
		Metadata: map[string]domaincontent.MetadataValue{
			"payload_ru": {Value: map[string]any{"title": "Курс", "price": 39900.0, "kicker": "Backend"}},
			"payload_en": {Value: map[string]any{"title": "Course", "price": 39900.0}},
		},
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if len(form.Issues) != 0 {
		t.Fatalf("issues: %#v", form.Issues)
	}
	if form.Extra["ru"]["kicker"] != "Backend" {
		t.Fatalf("extra lost: %#v", form.Extra)
	}
}

func TestRecordProjectsNestedObjectFields(t *testing.T) {
	t.Parallel()
	resource := schema.Resource{
		ID: "product", Collection: "products",
		Form: []schema.Field{{
			ID: "author", Type: schema.FieldObject,
			Fields: []schema.Field{
				{ID: "name", Type: schema.FieldString},
				{ID: "links", Type: schema.FieldCollection, Items: &schema.Field{ID: "item", Type: schema.FieldString}},
			},
		}},
	}
	record := Record(resource)
	if len(record.Fields) != 1 || record.Fields[0].Type != "object" || len(record.Fields[0].Fields) != 2 {
		t.Fatalf("projected fields: %#v", record.Fields)
	}
	if record.Fields[0].Fields[1].Items == nil || record.Fields[0].Fields[1].Items.Type != "text" {
		t.Fatalf("nested collection: %#v", record.Fields[0].Fields)
	}
}

func TestValidateEntryRejectsMissingRequiredFormField(t *testing.T) {
	t.Parallel()
	resource := schema.Resource{
		ID: "product", Collection: "products",
		Form: []schema.Field{{ID: "title", Type: schema.FieldString, Required: true}},
	}
	err := ValidateEntry(resource, domaincontent.Entry{
		Metadata: map[string]domaincontent.MetadataValue{
			"payload_en": {Value: map[string]any{"price": 10.0}},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateEntryRejectsNonObjectPayload(t *testing.T) {
	t.Parallel()
	err := ValidateEntry(schema.Resource{ID: "product", Collection: "products"}, domaincontent.Entry{
		Metadata: map[string]domaincontent.MetadataValue{
			"payload_ru": {Value: []any{"broken"}},
		},
	})
	if err == nil {
		t.Fatal("expected payload object error")
	}
}

func TestValidateEntrySkipsStorageFieldsUntilFormIsDeclared(t *testing.T) {
	t.Parallel()
	resource := schema.Resource{
		ID: "product", Collection: "products",
		Fields: []schema.Field{{ID: "price", Type: schema.FieldMoney, Required: true}},
	}
	if err := ValidateEntry(resource, domaincontent.Entry{
		Metadata: map[string]domaincontent.MetadataValue{"price": {Value: "49.95"}},
	}); err != nil {
		t.Fatalf("storage-only fields must not run formset validation: %v", err)
	}
}
