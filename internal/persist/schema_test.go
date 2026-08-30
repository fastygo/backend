package persist

import (
	"testing"

	"github.com/fastygo/backend/internal/domain/schema"
)

func TestManifestDocumentRoundTrip(t *testing.T) {
	t.Parallel()
	cases := map[string]schema.Manifest{
		"store": {
			Name: "store", Version: "1",
			Resources: []schema.Resource{{
				ID: "product", Collection: "products", Public: true,
				Fields: []schema.Field{
					{ID: "title", Type: schema.FieldString, Required: true, Localized: true},
					{ID: "brand", Type: schema.FieldRelation, Relation: &schema.Relation{
						Resource: "product", Cardinality: schema.CardinalityOne, OnDelete: schema.DeleteRestrict,
					}},
				},
				Form: []schema.Field{{ID: "price", Type: schema.FieldMoney}, {
					ID: "author", Type: schema.FieldObject,
					Fields: []schema.Field{{ID: "name", Type: schema.FieldString}, {ID: "role", Type: schema.FieldString}},
				}, {ID: "body", Type: schema.FieldRichText, Localized: true}},
			}},
		},
	}
	for name, manifest := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded, err := EncodeManifest(manifest)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := DecodeManifest(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			want, err := ManifestDigest(manifest)
			if err != nil {
				t.Fatalf("digest source: %v", err)
			}
			got, err := ManifestDigest(decoded)
			if err != nil {
				t.Fatalf("digest decoded: %v", err)
			}
			if want != got {
				t.Fatalf("persist manifest changed digest: %s vs %s", want, got)
			}
		})
	}
}

func TestManifestDigestIsOrderIndependent(t *testing.T) {
	t.Parallel()
	first := schema.Manifest{
		Name: "store", Version: "1.0.0",
		Resources: []schema.Resource{
			{ID: "category", Collection: "categories", Public: true, Fields: []schema.Field{
				{ID: "name", Type: schema.FieldString, Required: true, Localized: true},
			}},
			{ID: "product", Collection: "products", Public: true, Fields: []schema.Field{
				{ID: "title", Type: schema.FieldString, Required: true},
				{ID: "price", Type: schema.FieldMoney, Required: true},
			}},
		},
	}
	second := first
	second.Resources = append([]schema.Resource(nil), first.Resources...)
	second.Resources[0], second.Resources[1] = second.Resources[1], second.Resources[0]
	second.Resources[0].Fields = append([]schema.Field(nil), second.Resources[0].Fields...)
	second.Resources[0].Fields[0], second.Resources[0].Fields[1] = second.Resources[0].Fields[1], second.Resources[0].Fields[0]
	left, err := ManifestDigest(first)
	if err != nil {
		t.Fatalf("digest first: %v", err)
	}
	right, err := ManifestDigest(second)
	if err != nil {
		t.Fatalf("digest second: %v", err)
	}
	if left != right {
		t.Fatalf("digest depends on declaration order")
	}
}
