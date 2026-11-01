package platform

import (
	"github.com/fastygo/backend/internal/domain/authz"
	domainschema "github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/panel"
)

// RegisterManifest projects domain schemas into UI-neutral Panel descriptors.
func (controlPlane *ControlPlane) RegisterManifest(manifest domainschema.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	resources := make([]panel.Resource[Capability], 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, projectResource(resource))
	}
	return controlPlane.Panel.AddResources(resources...)
}

// projectResource builds the panel resource from the Codex record with
// panel.ResourceFromRecord; only entry chrome (title, slug, status,
// visibility, updated_at) is added here.
func projectResource(resource domainschema.Resource) panel.Resource[Capability] {
	basePath := "/go-admin/resources/" + resource.ID
	projected := panel.ResourceFromRecord(resource.Record(), panel.ResourceOptions[Capability]{
		BasePath: basePath,
		Icon:     "database",
		Capabilities: map[panel.ResourceOperation]Capability{
			panel.OperationList:   authz.CapabilityContentReadPrivate,
			panel.OperationCreate: authz.CapabilityContentCreate,
			panel.OperationEdit:   authz.CapabilityContentEdit,
			panel.OperationDelete: authz.CapabilityContentDelete,
		},
		LeadColumns: []panel.Column{
			{ID: "title", Label: "Title", Type: panel.ColumnText, Sortable: true, Searchable: true},
			{ID: "status", Label: "Status", Type: panel.ColumnBadge, Sortable: true},
		},
		TrailColumns: []panel.Column{{ID: "updated_at", Label: "Updated", Type: panel.ColumnDateTime, Sortable: true}},
	})
	chrome := []panel.Field{
		{ID: "title", Label: "Title", Type: panel.FieldText, Required: true},
		{ID: "slug", Label: "Slug", Type: panel.FieldText, Required: true},
		{ID: "status", Label: "Status", Type: panel.FieldStatus, Required: true},
		{ID: "visibility", Label: "Visibility", Type: panel.FieldSelect, Required: true},
	}
	fields := append(append([]panel.Field(nil), chrome...), panel.FieldsFromRecord(domainschema.Resource{
		ID: resource.ID, Fields: resource.Fields, Relations: resource.Relations,
	}.Record())...)
	projected.Label = domainschema.HumanLabel(resource.ID)
	projected.Singular = projected.Label
	projected.Plural = domainschema.HumanLabel(resource.Collection)
	projected.Navigation = panel.MenuItem[Capability]{
		ID: resource.ID, Label: projected.Plural, Path: basePath,
		Capability: authz.CapabilityAdminAccess,
	}
	projected.Table.Searchable = true
	projected.Table.Exportable = true
	projected.Table.PerPage = []int{20, 50, 100}
	projected.Form = panel.FormSchema{Fields: fields}
	projected.Detail = panel.DetailSchema{Fields: fields}
	return projected
}
