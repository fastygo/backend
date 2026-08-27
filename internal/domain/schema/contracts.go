package schema

import (
	"errors"
	"sort"
	"strings"
	"unicode"
)

const (
	payloadRU = "payload_ru"
	payloadEN = "payload_en"
)

func (manifest Manifest) Resource(id string) (Resource, bool) {
	for _, resource := range manifest.Resources {
		if resource.ID == id {
			return resource, true
		}
	}
	return Resource{}, false
}

// FormFields is the slot schema: explicit Form, otherwise storage fields minus locale blobs.
func (resource Resource) FormFields() []Field {
	if len(resource.Form) > 0 {
		return append([]Field(nil), resource.Form...)
	}
	fields := make([]Field, 0, len(resource.Fields))
	for _, field := range resource.Fields {
		if field.ID == payloadRU || field.ID == payloadEN {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

func (manifest Manifest) JSONSchema(resourceID string) (map[string]any, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	resource, exists := manifest.Resource(resourceID)
	if !exists {
		return nil, errors.New("resource does not exist")
	}
	properties := map[string]any{
		"status":     map[string]any{"type": "string", "enum": []string{"draft", "scheduled", "published", "archived", "trashed"}},
		"visibility": map[string]any{"type": "string", "enum": []string{"public", "private"}},
	}
	required := []string{"status", "visibility"}
	for _, field := range resource.Fields {
		projected := fieldJSONSchema(field)
		properties[field.ID] = projected
		if field.Required {
			required = append(required, field.ID)
		}
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "headless://" + manifest.Name + "/" + manifest.Version + "/" + resource.ID,
		"title":                graphQLName(resource.ID),
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}, nil
}

func (manifest Manifest) GraphQLSDL() (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	resources := append([]Resource(nil), manifest.Resources...)
	sort.Slice(resources, func(left, right int) bool { return resources[left].ID < resources[right].ID })
	var result strings.Builder
	result.WriteString("scalar JSON\nscalar DateTime\n\n")
	result.WriteString("type SchemaIdentity { name: String!, version: String!, digest: String! }\n")
	result.WriteString("type FormsetIssue { locale: String, field: String, code: String!, message: String! }\n")
	result.WriteString("type FormsetForm { record: String!, locales: [String!]!, fields: JSON!, values: JSON!, extra: JSON, issues: [FormsetIssue!]!, schema: JSON!, payloads: JSON! }\n\n")
	result.WriteString("type Query {\n  schemaIdentity: SchemaIdentity!\n")
	result.WriteString("  formsetSchema(resource: ID!): FormsetForm!\n")
	result.WriteString("  formset(resource: ID!, id: ID!): FormsetForm!\n")
	for _, resource := range resources {
		name := graphQLName(resource.ID)
		result.WriteString("  " + graphQLField(resource.Collection) + "(page: Int = 1, perPage: Int = 20, search: String, status: String, locale: String): " + name + "Page!\n")
		result.WriteString("  " + graphQLField(resource.ID) + "(id: ID!): " + name + "\n")
	}
	result.WriteString("}\n\n")
	result.WriteString("type Mutation {\n")
	for _, resource := range resources {
		name := graphQLName(resource.ID)
		result.WriteString("  create" + name + "(input: " + name + "Input!, idempotencyKey: String): " + name + "!\n")
		result.WriteString("  update" + name + "(id: ID!, input: " + name + "Input!, expectedVersion: Int!): " + name + "!\n")
		result.WriteString("  delete" + name + "(id: ID!, expectedVersion: Int!): Boolean!\n")
	}
	result.WriteString("}\n\n")
	for _, resource := range resources {
		writeGraphQLResource(&result, resource)
	}
	return result.String(), nil
}

func (manifest Manifest) OpenAPI() (map[string]any, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	paths := map[string]any{
		"/healthz": map[string]any{"get": operation("liveness", "Liveness probe", "200")},
		"/readyz":  map[string]any{"get": operation("readiness", "Readiness probe", "200")},
		"/go-json/go/v2/": map[string]any{
			"get": operation("discoverCodex", "Discover Codex v2 routes", "200"),
		},
		"/go-json/go/v2/types": map[string]any{
			"get": operation("listContentTypes", "List kinds, collections, and forms", "200"),
		},
		"/go-json/go/v2/audit": map[string]any{
			"get": operation("listAuditEvents", "List audit events", "200"),
		},
		"/go-json/go/v2/media": map[string]any{
			"get":  operation("listMedia", "List media", "200"),
			"post": operation("uploadMedia", "Upload media", "201"),
		},
		"/go-json/go/v2/media/{id}": map[string]any{
			"get":    operation("getMedia", "Get media", "200"),
			"patch":  operation("updateMedia", "Update media", "200"),
			"delete": operation("trashMedia", "Move media to trash", "204"),
		},
		"/go-json/go/v2/media/{id}/content": map[string]any{
			"get": operation("downloadMedia", "Download media", "200"),
		},
		"/go-json/go/v2/taxonomies": map[string]any{
			"get":  operation("listTaxonomies", "List taxonomies", "200"),
			"post": operation("createTaxonomy", "Create taxonomy", "201"),
		},
		"/go-json/go/v2/taxonomies/{taxonomy}": map[string]any{
			"get":    operation("listTaxonomyTerms", "List taxonomy terms", "200"),
			"put":    operation("updateTaxonomy", "Update taxonomy", "200"),
			"delete": operation("deleteTaxonomy", "Delete taxonomy", "204"),
		},
		"/go-json/go/v2/taxonomies/{taxonomy}/terms": map[string]any{
			"post": operation("createTaxonomyTerm", "Create taxonomy term", "201"),
		},
		"/go-json/go/v2/taxonomies/{taxonomy}/terms/{term}": map[string]any{
			"put":    operation("updateTaxonomyTerm", "Update taxonomy term", "200"),
			"delete": operation("deleteTaxonomyTerm", "Delete taxonomy term", "204"),
		},
		"/go-json/go/v2/auth/login": map[string]any{
			"post": operation("login", "Authenticate user", "200"),
		},
		"/go-json/go/v2/users": map[string]any{
			"get":  operation("listUsers", "List users", "200"),
			"post": operation("createUser", "Create user", "201"),
		},
		"/go-json/go/v2/users/{id}": map[string]any{
			"put":    operation("updateUser", "Update user", "200"),
			"delete": operation("deleteUser", "Delete user", "204"),
		},
		"/go-json/go/v2/roles": map[string]any{
			"get":  operation("listRoles", "List roles", "200"),
			"post": operation("createRole", "Create role", "201"),
		},
		"/go-json/go/v2/roles/{id}": map[string]any{
			"put":    operation("updateRole", "Update role", "200"),
			"delete": operation("deleteRole", "Delete role", "204"),
		},
	}
	schemas := map[string]any{}
	for _, resource := range manifest.Resources {
		resourceSchema, err := manifest.JSONSchema(resource.ID)
		if err != nil {
			return nil, err
		}
		name := graphQLName(resource.ID)
		schemas[name+"Values"] = resourceSchema
		if !resource.RegistersCodexCollection() {
			continue
		}
		collectionPath := "/go-json/go/v2/" + resource.Collection
		recordPath := collectionPath + "/{id}"
		paths[collectionPath] = map[string]any{
			"get":  operation("list"+graphQLName(resource.Collection), "List "+resource.Collection, "200"),
			"post": operation("create"+name, "Create "+resource.ID, "201"),
		}
		paths[recordPath] = map[string]any{
			"get":    operation("get"+name, "Get "+resource.ID, "200"),
			"patch":  operation("update"+name, "Update "+resource.ID, "200"),
			"delete": operation("trash"+name, "Move "+resource.ID+" to trash", "204"),
		}
		paths[recordPath+"/transitions"] = map[string]any{
			"post": operation("transition"+name, "Transition "+resource.ID+" lifecycle", "200"),
		}
		paths[recordPath+"/revisions"] = map[string]any{
			"get": operation("list"+name+"Revisions", "List "+resource.ID+" revisions", "200"),
		}
		paths[recordPath+"/revisions/{revision}/restore"] = map[string]any{
			"post": operation("restore"+name+"Revision", "Restore "+resource.ID+" revision", "200"),
		}
		paths[recordPath+"/form"] = map[string]any{
			"get": operation("get"+name+"Form", "Get "+resource.ID+" form", "200"),
		}
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title": manifest.Name + " Headless API", "version": manifest.Version,
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": schemas,
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
			},
		},
	}, nil
}

func fieldJSONSchema(field Field) map[string]any {
	projected := map[string]any{"type": jsonType(field.Type)}
	if field.Nullable {
		projected["type"] = []string{jsonType(field.Type), "null"}
	}
	if field.ReadOnly {
		projected["readOnly"] = true
	}
	if field.Sensitive {
		projected["writeOnly"] = true
	}
	if len(field.Enum) > 0 {
		projected["enum"] = field.Enum
	}
	if field.Localized {
		projected = map[string]any{
			"type": "object", "additionalProperties": false,
			"patternProperties": map[string]any{
				"^[a-z]{2,3}(-[A-Z]{2})?$": projected,
			},
		}
	}
	if field.Relation != nil {
		projected["x-resource"] = field.Relation.Resource
		projected["x-on-delete"] = field.Relation.OnDelete
	}
	if field.Type == FieldCollection && field.Items != nil {
		projected["items"] = fieldJSONSchema(*field.Items)
	}
	return projected
}

func jsonType(fieldType FieldType) string {
	switch fieldType {
	case FieldBoolean:
		return "boolean"
	case FieldInteger:
		return "integer"
	case FieldNumber, FieldDecimal, FieldMoney:
		return "number"
	case FieldJSON:
		return "object"
	case FieldCollection:
		return "array"
	default:
		return "string"
	}
}

func writeGraphQLResource(result *strings.Builder, resource Resource) {
	name := graphQLName(resource.ID)
	result.WriteString("type " + name + " {\n")
	result.WriteString("  id: ID!\n  version: Int!\n  title: String!\n  slug: String!\n")
	result.WriteString("  titleLocalized: JSON\n  slugLocalized: JSON\n  contentLocalized: JSON\n  excerptLocalized: JSON\n")
	result.WriteString("  status: String!\n  visibility: String!\n  payloadRu: JSON\n  payloadEn: JSON\n  createdAt: DateTime!\n  updatedAt: DateTime!\n")
	for _, field := range resource.Fields {
		if !field.Sensitive {
			result.WriteString("  " + graphQLField(field.ID) + ": " + graphQLType(field, false) + "\n")
		}
	}
	result.WriteString("}\n\ninput " + name + "Input {\n")
	result.WriteString("  title: String\n  slug: String\n  titleLocalized: JSON\n  slugLocalized: JSON\n")
	result.WriteString("  contentLocalized: JSON\n  excerptLocalized: JSON\n  status: String\n  visibility: String\n  payloadRu: JSON\n  payloadEn: JSON\n")
	for _, field := range resource.Fields {
		if !field.ReadOnly {
			result.WriteString("  " + graphQLField(field.ID) + ": " + graphQLType(field, field.Required && !field.Nullable) + "\n")
		}
	}
	result.WriteString("}\n\ntype " + name + "Page { items: [" + name + "!]!, page: Int!, perPage: Int!, total: Int!, totalPages: Int! }\n\n")
}

func graphQLType(field Field, required bool) string {
	var resolved string
	switch field.Type {
	case FieldBoolean:
		resolved = "Boolean"
	case FieldInteger:
		resolved = "Int"
	case FieldNumber, FieldDecimal, FieldMoney:
		resolved = "Float"
	case FieldUUID, FieldRelation, FieldMedia:
		resolved = "ID"
	case FieldJSON, FieldCollection:
		resolved = "JSON"
	case FieldDate, FieldDateTime:
		resolved = "DateTime"
	default:
		resolved = "String"
	}
	if field.Localized {
		resolved = "JSON"
	}
	if field.Relation != nil && field.Relation.Cardinality == CardinalityMany {
		resolved = "[" + resolved + "!]"
	}
	if required {
		resolved += "!"
	}
	return resolved
}

func graphQLName(identifier string) string {
	parts := strings.FieldsFunc(identifier, func(character rune) bool {
		return character == '_' || character == '-'
	})
	for index, part := range parts {
		letters := []rune(part)
		if len(letters) > 0 {
			letters[0] = unicode.ToUpper(letters[0])
		}
		parts[index] = string(letters)
	}
	return strings.Join(parts, "")
}

func graphQLField(identifier string) string {
	name := graphQLName(identifier)
	if name == "" {
		return ""
	}
	letters := []rune(name)
	letters[0] = unicode.ToLower(letters[0])
	return string(letters)
}

func operation(id, description, status string) map[string]any {
	return map[string]any{
		"operationId": id, "summary": description,
		"responses": map[string]any{status: map[string]any{"description": description}},
	}
}
