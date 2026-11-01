package platform

import (
	"strings"
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
					{ID: "price", Type: schema.FieldNumber, Rules: schema.Rules(schema.RuleMoney), Required: true},
					{ID: "category", Type: schema.FieldRelation, Indexed: true},
					{ID: "sku", Type: schema.FieldString, Indexed: true, Searchable: true},
					{ID: "secret", Type: schema.FieldString, Indexed: true, Sensitive: true},
				},
				Relations: []schema.Relation{{ID: "category", Target: "category", Cardinality: schema.OneToOne, DeleteBehavior: schema.DeleteRestrict}},
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
	if product.Form.Fields[4].DataType != schema.FieldNumber || product.Form.Fields[4].Rules[0].Name != schema.RuleMoney {
		t.Fatalf("codex data type lost: %#v", product.Form.Fields[4])
	}
	columns := []string{}
	for _, column := range product.Table.Columns {
		columns = append(columns, column.ID)
	}
	if got := strings.Join(columns, ","); got != "title,status,category,sku,updated_at" {
		t.Fatalf("columns = %s (manifest columns must come from Indexed/Searchable, never sensitive)", got)
	}
	if product.Plural != "Products" || product.BasePath != "/go-admin/resources/product" {
		t.Fatalf("labels: %#v", product)
	}
	if product.Capabilities[1].Capability != authz.CapabilityContentCreate {
		t.Fatalf("RBAC capability was not projected")
	}
}
