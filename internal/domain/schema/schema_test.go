package schema

import "testing"

func TestManifestValidation(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		mutate    func(*Manifest)
		wantError bool
	}{
		"valid commerce schema": {mutate: func(*Manifest) {}},
		"missing relation target": {
			mutate:    func(manifest *Manifest) { manifest.Resources[1].Fields[1].Relation.Resource = "missing" },
			wantError: true,
		},
		"collection without items": {
			mutate: func(manifest *Manifest) {
				manifest.Resources[1].Fields = append(manifest.Resources[1].Fields, Field{
					ID: "variants", Type: FieldCollection,
				})
			},
			wantError: true,
		},
		"reserved rest_base": {
			mutate: func(manifest *Manifest) {
				manifest.Resources = append(manifest.Resources, Resource{
					ID: "lead", Collection: "search", RESTVisible: true,
				})
			},
			wantError: true,
		},
		"lead collection": {
			mutate: func(manifest *Manifest) {
				manifest.Resources = append(manifest.Resources, Resource{
					ID: "lead", Collection: "leads", Public: false, RESTVisible: true,
				})
			},
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			manifest := productManifest()
			test.mutate(&manifest)
			err := manifest.Validate()
			if test.wantError && err == nil {
				t.Fatalf("expected validation error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid manifest rejected: %v", err)
			}
		})
	}
}

func TestWithCoreResourcesKeepsSiteKinds(t *testing.T) {
	t.Parallel()
	manifest := WithCoreResources(Manifest{
		Name: "gitcourse", Version: "1",
		Resources: []Resource{
			{ID: "product", Collection: "products", Public: true},
			{ID: "post", Collection: "posts", Public: true, Form: []Field{{ID: "content", Type: FieldText}}},
		},
	})
	if _, ok := manifest.Resource("product"); !ok {
		t.Fatal("site kind was dropped")
	}
	post, ok := manifest.Resource("post")
	if !ok || len(post.Form) != 1 || post.Form[0].ID != "content" {
		t.Fatalf("declared post form was replaced: %#v", post)
	}
	if _, ok := manifest.Resource("page"); !ok {
		t.Fatal("core page kind is required")
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("merged manifest: %v", err)
	}
}

func TestRegistersCodexCollectionFollowsShowInRest(t *testing.T) {
	t.Parallel()
	lead := Resource{ID: "lead", Collection: "leads", RESTVisible: true}
	if !lead.RegistersCodexCollection() {
		t.Fatal("lead with rest_visible must register /go/v2/leads")
	}
	hidden := Resource{ID: "note", Collection: "notes", RESTVisible: false}
	if hidden.RegistersCodexCollection() {
		t.Fatal("rest_visible false must not register a collection route")
	}
	menu := Resource{ID: "menu", Collection: "menus", RESTVisible: true}
	if menu.RegistersCodexCollection() {
		t.Fatal("menus stay on the dedicated controller")
	}
}

func TestFormFieldsPrefersExplicitFormOverPayloadBlobs(t *testing.T) {
	t.Parallel()
	resource := Resource{
		ID: "product", Collection: "products",
		Fields: []Field{{ID: "payload_ru", Type: FieldJSON}, {ID: "payload_en", Type: FieldJSON}},
		Form:   []Field{{ID: "title", Type: FieldString, Required: true}},
	}
	fields := resource.FormFields()
	if len(fields) != 1 || fields[0].ID != "title" {
		t.Fatalf("form fields=%#v", fields)
	}
}

func TestManifestSupportsLocalizedCommerceSchema(t *testing.T) {
	t.Parallel()
	manifest := productManifest()
	product := manifest.Resources[1]
	if !product.Fields[0].Localized || product.Fields[1].Relation.Cardinality != CardinalityMany {
		t.Fatalf("localized or relation contract was lost")
	}
}

func productManifest() Manifest {
	return Manifest{
		Name:    "store",
		Version: "1.0.0",
		Resources: []Resource{
			{
				ID: "category", Collection: "categories", Public: true, RESTVisible: true,
				Fields: []Field{
					{ID: "name", Type: FieldString, Required: true, Localized: true},
				},
			},
			{
				ID: "product", Collection: "products", Public: true, RESTVisible: true, GraphQLVisible: true,
				Taxonomies: []string{"brand", "collection"},
				Fields: []Field{
					{ID: "title", Type: FieldString, Required: true, Localized: true},
					{
						ID: "categories", Type: FieldRelation,
						Relation: &Relation{Resource: "category", Cardinality: CardinalityMany, OnDelete: DeleteRestrict},
					},
					{ID: "price", Type: FieldMoney, Required: true},
				},
			},
		},
	}
}
