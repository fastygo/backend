package platform

import (
	"strings"
	"unicode"

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

func projectResource(resource domainschema.Resource) panel.Resource[Capability] {
	label := labelFor(resource.ID)
	fields := []panel.Field{
		{ID: "title", Label: "Title", Type: panel.FieldText, Required: true},
		{ID: "slug", Label: "Slug", Type: panel.FieldText, Required: true},
		{ID: "status", Label: "Status", Type: panel.FieldStatus, Required: true},
		{ID: "visibility", Label: "Visibility", Type: panel.FieldSelect, Required: true},
	}
	columns := []panel.Column{
		{ID: "title", Label: "Title", Type: panel.ColumnText, Sortable: true, Searchable: true},
		{ID: "status", Label: "Status", Type: panel.ColumnBadge, Sortable: true},
		{ID: "updated_at", Label: "Updated", Type: panel.ColumnDateTime, Sortable: true},
	}
	relations := make([]panel.ResourceRelation, 0)
	for _, field := range resource.Fields {
		fields = append(fields, projectField(field))
		if field.Relation != nil {
			relations = append(relations, panel.ResourceRelation{
				ID: field.ID, Label: labelFor(field.ID),
				ResourceID: panel.ResourceID(field.Relation.Resource), Type: string(field.Relation.Cardinality),
			})
		}
		if len(columns) < 7 && !field.Sensitive {
			columns = append(columns, panel.Column{
				ID: field.ID, Label: labelFor(field.ID), Type: projectColumnType(field.Type),
				Sortable:   field.Type != domainschema.FieldJSON && field.Type != domainschema.FieldCollection,
				Searchable: field.Type == domainschema.FieldString || field.Type == domainschema.FieldText,
				Toggleable: true,
			})
		}
	}
	basePath := "/go-admin/resources/" + resource.ID
	return panel.Resource[Capability]{
		ID: panel.ResourceID(resource.ID), Label: label, Singular: label, Plural: labelFor(resource.Collection),
		BasePath: basePath, Icon: "database",
		Navigation: panel.MenuItem[Capability]{
			ID: resource.ID, Label: labelFor(resource.Collection), Path: basePath,
			Capability: authz.CapabilityAdminAccess,
		},
		Capabilities: []panel.ResourceCapability[Capability]{
			{Operation: panel.OperationList, Capability: authz.CapabilityContentReadPrivate},
			{Operation: panel.OperationCreate, Capability: authz.CapabilityContentCreate},
			{Operation: panel.OperationEdit, Capability: authz.CapabilityContentEdit},
			{Operation: panel.OperationDelete, Capability: authz.CapabilityContentDelete},
		},
		Table: panel.TableSchema[Capability]{
			Columns: columns, Searchable: true, Exportable: true, PerPage: []int{20, 50, 100},
		},
		Form:      panel.FormSchema{Fields: fields},
		Detail:    panel.DetailSchema{Fields: fields},
		Relations: relations,
	}
}

func projectField(field domainschema.Field) panel.Field {
	projected := panel.Field{
		ID: field.ID, Label: labelFor(field.ID), Type: projectFieldType(field.Type),
		Required: field.Required, ReadOnly: field.ReadOnly,
	}
	for _, value := range field.Enum {
		projected.Options = append(projected.Options, panel.Option{Value: value, Label: labelFor(value)})
	}
	if field.Relation != nil {
		projected.Relation = panel.Relation{
			ID: field.ID, Label: projected.Label, ResourceID: field.Relation.Resource,
			DisplayColumn: "title", Multiple: field.Relation.Cardinality == domainschema.CardinalityMany,
			Searchable: true,
		}
	}
	if field.Items != nil {
		projected.Fields = []panel.Field{projectField(*field.Items)}
	}
	return projected
}

func projectFieldType(fieldType domainschema.FieldType) panel.FieldType {
	switch fieldType {
	case domainschema.FieldText:
		return panel.FieldTextarea
	case domainschema.FieldBoolean:
		return panel.FieldBoolean
	case domainschema.FieldInteger, domainschema.FieldNumber, domainschema.FieldDecimal, domainschema.FieldMoney:
		return panel.FieldNumber
	case domainschema.FieldDate, domainschema.FieldDateTime:
		return panel.FieldDateTime
	case domainschema.FieldEnum:
		return panel.FieldSelect
	case domainschema.FieldRelation:
		return panel.FieldRelation
	case domainschema.FieldCollection:
		return panel.FieldRepeater
	case domainschema.FieldMedia:
		return panel.FieldFile
	case domainschema.FieldJSON:
		return panel.FieldJSON
	default:
		return panel.FieldText
	}
}

func projectColumnType(fieldType domainschema.FieldType) panel.ColumnType {
	switch fieldType {
	case domainschema.FieldBoolean:
		return panel.ColumnBoolean
	case domainschema.FieldInteger, domainschema.FieldNumber, domainschema.FieldDecimal, domainschema.FieldMoney:
		return panel.ColumnNumber
	case domainschema.FieldDate, domainschema.FieldDateTime:
		return panel.ColumnDateTime
	case domainschema.FieldMedia:
		return panel.ColumnImage
	case domainschema.FieldEnum:
		return panel.ColumnBadge
	default:
		return panel.ColumnText
	}
}

func labelFor(identifier string) string {
	identifier = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(identifier, "_", " "), "-", " "))
	if identifier == "" {
		return ""
	}
	letters := []rune(identifier)
	letters[0] = unicode.ToUpper(letters[0])
	return string(letters)
}
