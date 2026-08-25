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
