package platform

import (
	"testing"

	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/panel"
)

func TestManifestProjectsToPanelDescriptors(t *testing.T) {
	controlPlane, err := NewControlPlane("headless", "Content", "/go-admin")
	if err != nil {
		t.Fatalf("create control plane: %v", err)
	}
	manifest := schema.Manifest{
		Name: "shop", Version: "1",
		Resources: []schema.Resource{
			{
				ID: "category", Collection: "categories",
				Fields: []schema.Field{{ID: "name", Type: schema.FieldString, Required: true}},
			},
			{
				ID: "product", Collection: "products",
				Fields: []schema.Field{
					{ID: "price", Type: schema.FieldMoney, Required: true},
					{
						ID: "category", Type: schema.FieldRelation,
						Relation: &schema.Relation{
							Resource: "category", Cardinality: schema.CardinalityOne, OnDelete: schema.DeleteRestrict,
						},
					},
				},
			},
		},
	}
	if err := controlPlane.RegisterManifest(manifest); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	resources := controlPlane.Panel.Resources()
	if len(resources) != 2 || resources[1].ID != panel.ResourceID("product") {
		t.Fatalf("manifest resources were not projected")
	}
	product := resources[1]
	if len(product.Relations) != 1 || product.Relations[0].ResourceID != panel.ResourceID("category") {
		t.Fatalf("relation descriptor was not projected")
	}
	if product.Form.Fields[4].Type != panel.FieldNumber || product.Form.Fields[5].Type != panel.FieldRelation {
		t.Fatalf("field descriptors were not projected")
	}
	if product.Capabilities[1].Capability != authz.CapabilityContentCreate {
		t.Fatalf("RBAC capability was not projected")
	}
}
