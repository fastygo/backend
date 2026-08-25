package schema

import "testing"

func TestManifestDigestIsOrderIndependent(t *testing.T) {
	first := productManifest()
	second := productManifest()
	second.Resources[1].Fields[0], second.Resources[1].Fields[1] = second.Resources[1].Fields[1], second.Resources[1].Fields[0]
	second.Resources[0], second.Resources[1] = second.Resources[1], second.Resources[0]

	firstDigest, err := first.Digest()
	if err != nil {
		t.Fatalf("digest first manifest: %v", err)
	}
	secondDigest, err := second.Digest()
	if err != nil {
		t.Fatalf("digest second manifest: %v", err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("manifest digest depends on declaration order")
	}
}

func TestManifestRejectsInvalidRelationAndCollection(t *testing.T) {
	manifest := productManifest()
	manifest.Resources[1].Fields[1].Relation.Resource = "missing"
	if err := manifest.Validate(); err == nil {
		t.Fatalf("missing relation target must fail")
	}

	manifest = productManifest()
	manifest.Resources[1].Fields = append(manifest.Resources[1].Fields, Field{
		ID: "variants", Type: FieldCollection,
	})
	if err := manifest.Validate(); err == nil {
		t.Fatalf("collection without item schema must fail")
	}
}

func TestManifestSupportsLocalizedCommerceSchema(t *testing.T) {
	manifest := productManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid commerce manifest rejected: %v", err)
	}
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
