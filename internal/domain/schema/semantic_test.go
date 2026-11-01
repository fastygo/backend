package schema

import (
	"reflect"
	"strings"
	"testing"
)

func TestSemanticJSONAndGraphQLProjection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		field    Field
		semantic Semantic
		json     string
		format   string
		scalar   string
	}{
		{Field{Type: FieldNumber, Rules: Rules(RuleMoney)}, SemanticMoney, "integer", "", "Int"},
		{Field{Type: FieldNumber, Rules: Rules(RuleInteger)}, SemanticInteger, "integer", "", "Int"},
		{Field{Type: FieldNumber, Rules: Rules(RuleDecimal)}, SemanticDecimal, "number", "", "Float"},
		{Field{Type: FieldNumber}, SemanticNumber, "number", "", "Float"},
		{Field{Type: FieldDateTime, Rules: Rules(RuleDate)}, SemanticDate, "string", "date", "DateTime"},
		{Field{Type: FieldDateTime}, SemanticDateTime, "string", "date-time", "DateTime"},
		{Field{Type: FieldString, Rules: Rules(RuleURI)}, SemanticURI, "string", "uri", "String"},
		{Field{Type: FieldString, Rules: Rules(RuleUUID)}, SemanticUUID, "string", "uuid", "ID"},
		{Field{Type: FieldString}, SemanticString, "string", "", "String"},
		{Field{Type: FieldText}, SemanticText, "string", "", "String"},
		{Field{Type: FieldTextarea}, SemanticText, "string", "", "String"},
		{Field{Type: FieldRichText}, SemanticRichText, "string", "", "String"},
		{Field{Type: FieldMarkdown}, SemanticMarkdown, "string", "", "String"},
		{Field{Type: FieldBoolean}, SemanticBoolean, "boolean", "", "Boolean"},
		{Field{Type: FieldSelect}, SemanticEnum, "string", "", "String"},
		{Field{Type: FieldJSON}, SemanticJSON, "object", "", "JSON"},
		{Field{Type: FieldObject}, SemanticObject, "object", "", "JSON"},
		{Field{Type: FieldCollection}, SemanticCollection, "array", "", "JSON"},
		{Field{Type: FieldRelation}, SemanticRelation, "string", "", "ID"},
		{Field{Type: FieldRelation, UIHint: UIHintMedia}, SemanticMedia, "string", "", "ID"},
		{Field{Type: FieldComputed}, SemanticComputed, "string", "", "String"},
		{Field{Type: FieldEncrypted}, SemanticEncrypted, "string", "", "String"},
		{Field{Type: "unknown"}, SemanticString, "string", "", "String"},
	}
	for _, test := range cases {
		if got := SemanticOf(test.field); got != test.semantic {
			t.Errorf("%s: semantic %s want %s", test.field.Type, got, test.semantic)
		}
		if got := jsonType(test.semantic); got != test.json {
			t.Errorf("%s: json type %s want %s", test.semantic, got, test.json)
		}
		if got := jsonFormat(test.semantic); got != test.format {
			t.Errorf("%s: format %s want %s", test.semantic, got, test.format)
		}
		if got := GraphQLScalar(test.field); got != test.scalar {
			t.Errorf("%s: scalar %s want %s", test.semantic, got, test.scalar)
		}
	}
	if GraphQLScalar(Field{Type: FieldNumber, Localized: true}) != "JSON" {
		t.Fatal("localized fields are JSON")
	}
}

func TestSemanticHelpers(t *testing.T) {
	t.Parallel()
	field := Field{ID: "state", Type: FieldSelect, Options: Options("a", "b"), Rules: Rules(RuleNullable, RuleReadOnly)}
	if !reflect.DeepEqual(EnumValues(field), []string{"a", "b"}) || field.Options[1].Label != "b" {
		t.Fatalf("options: %#v", field.Options)
	}
	if !Nullable(field) || !ReadOnly(field) || Nullable(Field{}) || ReadOnly(Field{}) {
		t.Fatal("rule helpers")
	}
	if OnDelete(Relation{}) != DeleteRestrict || OnDelete(Relation{DeleteBehavior: DeleteCascade}) != DeleteCascade {
		t.Fatal("on delete")
	}
	if Many(Relation{Cardinality: OneToOne}) || !Many(Relation{Cardinality: ManyToMany}) {
		t.Fatal("many")
	}
	if HumanLabel("  ") != "" || HumanLabel("sale_price") != "Sale price" {
		t.Fatal("human label")
	}
}

func richResource() Resource {
	return Resource{
		ID: "item", Collection: "items", Public: true,
		Taxonomies: []string{"z_tax", "a_tax"},
		Fields: []Field{
			{ID: "zeta", Type: FieldString, Rules: Rules(RuleNullable, RuleReadOnly)},
			{ID: "secret", Type: FieldString, Sensitive: true},
			{ID: "state", Type: FieldSelect, Options: Options("on", "off")},
			{ID: "tags", Type: FieldCollection, Items: &Field{ID: "tag", Type: FieldString}},
			{ID: "dims", Type: FieldObject, Fields: []Field{{ID: "w", Type: FieldNumber}, {ID: "h", Type: FieldNumber}}},
			{ID: "cover", Type: FieldRelation, UIHint: UIHintMedia},
			{ID: "related", Type: FieldRelation},
			{ID: "payload_en", Type: FieldJSON},
		},
		Relations: []Relation{{ID: "related", Target: "item", Cardinality: ManyToMany}},
	}
}

func TestResourceRecordFormAndCanonical(t *testing.T) {
	t.Parallel()
	resource := richResource()
	manifest := Manifest{Name: "m", Version: "1", Resources: []Resource{resource}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	record := resource.Record()
	if record.Label != "Item" || record.Visibility != "public" || len(record.Relations) != 1 ||
		record.Relations[0].Source != "item" || record.Relations[0].DeleteBehavior != DeleteRestrict {
		t.Fatalf("record: %#v", record)
	}
	if record.Fields[3].Items.Label != "Tag" || record.Fields[4].Fields[0].Label != "W" {
		t.Fatalf("nested labels: %#v", record.Fields[3])
	}
	if len(resource.FormFields()) != 7 {
		t.Fatalf("form fields skip payload_*: %d", len(resource.FormFields()))
	}
	form := resource.FormRecord()
	if len(form.Fields) != 7 {
		t.Fatalf("form record: %d", len(form.Fields))
	}
	explicit := resource
	explicit.Form = []Field{{ID: "title", Type: FieldString}}
	if fields := explicit.FormRecord().Fields; len(fields) != 1 || fields[0].Label != "Title" {
		t.Fatalf("explicit form: %#v", fields)
	}
	if (Resource{ID: "x", Label: "Custom"}).Record().Label != "Custom" || (Resource{ID: "x"}).Record().Relations != nil {
		t.Fatal("label override / nil relations")
	}

	canonical := Manifest{Name: "m", Version: "1", Resources: []Resource{{ID: "b"}, resource}}.Canonical()
	first := canonical.Resources[0]
	if first.ID != "b" || canonical.Resources[1].Fields[0].ID != "cover" || canonical.Resources[1].Taxonomies[0] != "a_tax" {
		t.Fatalf("canonical order: %#v", canonical.Resources)
	}
	dims := canonical.Resources[1].Fields[1]
	if dims.ID != "dims" || dims.Fields[0].ID != "h" {
		t.Fatalf("nested sorted: %#v", dims)
	}
	canonical.Resources[1].Fields[0].ID = "mutated"
	if resource.Fields[5].ID != "cover" {
		t.Fatal("canonical must deep copy")
	}

	jsonSchema, err := manifest.JSONSchema("item")
	if err != nil {
		t.Fatal(err)
	}
	properties := jsonSchema["properties"].(map[string]any)
	zeta := properties["zeta"].(map[string]any)
	if !reflect.DeepEqual(zeta["type"], []string{"string", "null"}) || zeta["readOnly"] != true || zeta["x-codex-rules"] == nil {
		t.Fatalf("zeta: %#v", zeta)
	}
	if properties["secret"].(map[string]any)["writeOnly"] != true || properties["state"].(map[string]any)["enum"] == nil {
		t.Fatal("secret/state")
	}
	if properties["tags"].(map[string]any)["items"] == nil || properties["dims"].(map[string]any)["properties"] == nil {
		t.Fatal("collection/object")
	}
	if properties["cover"].(map[string]any)["x-resource"] != MediaTarget || properties["related"].(map[string]any)["type"] != "array" {
		t.Fatal("media/related")
	}
	if _, err := manifest.JSONSchema("missing"); err == nil {
		t.Fatal("missing resource")
	}
	if _, err := (Manifest{}).JSONSchema("item"); err == nil {
		t.Fatal("invalid manifest")
	}
	sdl, err := manifest.GraphQLSDL()
	if err != nil || strings.Count(sdl, "secret:") != 1 || !strings.Contains(sdl, "related: [ID!]") {
		t.Fatalf("sdl: %v\n%s", err, sdl)
	}
}

func TestManifestValidationErrors(t *testing.T) {
	t.Parallel()
	valid := func() Resource { return Resource{ID: "a", Collection: "as"} }
	cases := map[string]Manifest{}
	add := func(name string, mutate func(*Manifest)) {
		manifest := Manifest{Name: "m", Version: "1", Resources: []Resource{valid()}}
		mutate(&manifest)
		cases[name] = manifest
	}
	add("no name", func(m *Manifest) { m.Name = "" })
	add("bad id", func(m *Manifest) { m.Resources[0].ID = "A B" })
	add("dup id", func(m *Manifest) { m.Resources = append(m.Resources, Resource{ID: "a", Collection: "bs"}) })
	add("dup collection", func(m *Manifest) { m.Resources = append(m.Resources, Resource{ID: "b", Collection: "as"}) })
	add("menus", func(m *Manifest) { m.Resources[0].Collection = "menus" })
	add("settings", func(m *Manifest) { m.Resources[0].Collection = "settings" })
	add("bad taxonomy", func(m *Manifest) { m.Resources[0].Taxonomies = []string{"Bad Tax"} })
	add("reserved field", func(m *Manifest) { m.Resources[0].Fields = []Field{{ID: "id", Type: FieldString}} })
	add("dup field", func(m *Manifest) {
		m.Resources[0].Fields = []Field{{ID: "x", Type: FieldString}, {ID: "x", Type: FieldString}}
	})
	add("legacy type", func(m *Manifest) { m.Resources[0].Fields = []Field{{ID: "x", Type: "money"}} })
	add("unknown type", func(m *Manifest) { m.Resources[0].Fields = []Field{{ID: "x", Type: "weird"}} })
	add("select no options", func(m *Manifest) { m.Resources[0].Fields = []Field{{ID: "x", Type: FieldSelect}} })
	add("bad item", func(m *Manifest) {
		m.Resources[0].Fields = []Field{{ID: "x", Type: FieldCollection, Items: &Field{ID: "i", Type: "money"}}}
	})
	add("bad nested", func(m *Manifest) {
		m.Resources[0].Fields = []Field{{ID: "x", Type: FieldObject, Fields: []Field{{ID: "n", Type: "weird"}}}}
	})
	add("bad form", func(m *Manifest) { m.Resources[0].Form = []Field{{ID: "x", Type: "weird"}} })
	rel := func(r Relation, fields ...Field) func(*Manifest) {
		return func(m *Manifest) { m.Resources[0].Relations = []Relation{r}; m.Resources[0].Fields = fields }
	}
	relField := Field{ID: "r", Type: FieldRelation}
	add("dup relation", func(m *Manifest) {
		r := Relation{ID: "r", Target: "a", Cardinality: OneToOne}
		m.Resources[0].Relations = []Relation{r, r}
		m.Resources[0].Fields = []Field{relField}
	})
	add("foreign source", rel(Relation{ID: "r", Source: "b", Target: "a", Cardinality: OneToOne}, relField))
	add("missing target", rel(Relation{ID: "r", Target: "zz", Cardinality: OneToOne}, relField))
	add("bad cardinality", rel(Relation{ID: "r", Target: "a", Cardinality: "weird"}, relField))
	add("bad delete", rel(Relation{ID: "r", Target: "a", Cardinality: OneToOne, DeleteBehavior: "weird"}, relField))
	add("unbound relation", rel(Relation{ID: "r", Target: "a", Cardinality: OneToOne}))
	add("undeclared relation field", func(m *Manifest) { m.Resources[0].Fields = []Field{relField} })
	add("media wrong target", rel(Relation{ID: "r", Target: "a", Cardinality: OneToOne}, Field{ID: "r", Type: FieldRelation, UIHint: UIHintMedia}))
	add("required nullify", rel(Relation{ID: "r", Target: "a", Cardinality: OneToOne, DeleteBehavior: DeleteNullify}, Field{ID: "r", Type: FieldRelation, Required: true}))
	for name, manifest := range cases {
		if err := manifest.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	ok := Manifest{Name: "m", Version: "1", Resources: []Resource{{ID: "menu", Collection: "menus"}}}
	if err := ok.Validate(); err != nil && !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("menu: %v", err)
	}
}
