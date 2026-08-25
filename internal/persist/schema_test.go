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
			want, err := manifest.Digest()
			if err != nil {
				t.Fatalf("digest source: %v", err)
			}
			got, err := decoded.Digest()
			if err != nil {
				t.Fatalf("digest decoded: %v", err)
			}
			if want != got {
				t.Fatalf("persist manifest changed digest: %s vs %s", want, got)
			}
		})
	}
}
