package graphqlapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/application/forms"
	"github.com/fastygo/backend/internal/domain/authz"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	domainschema "github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/formset"
	"github.com/google/uuid"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
)

const maxRequestBody = 4 << 20

type PrincipalResolver interface {
	Resolve(*http.Request) (authz.Principal, error)
}

type Handler struct {
	service   *application.Service
	principal PrincipalResolver
	schema    graphql.Schema
}

func New(service *application.Service, manifest domainschema.Manifest, principal PrincipalResolver) (*Handler, error) {
	if service == nil {
		return nil, errors.New("content service is required")
	}
	handler := &Handler{service: service, principal: principal}
	schema, err := handler.buildSchema(manifest)
	if err != nil {
		return nil, err
	}
	handler.schema = schema
	return handler, nil
}

func (handler *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /go-graphql", handler.serve)
	mux.HandleFunc("POST /go-graphql", handler.serve)
}

type cookieCSRFValidator interface {
	ValidateCookieCSRF(*http.Request) error
}

func (handler *Handler) serve(response http.ResponseWriter, request *http.Request) {
	document := graphQLRequest{
		Query:         request.URL.Query().Get("query"),
		OperationName: request.URL.Query().Get("operationName"),
	}
	if request.Method == http.MethodPost {
		request.Body = http.MaxBytesReader(response, request.Body, maxRequestBody)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&document); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{
				"errors": []map[string]any{{"message": "invalid GraphQL request"}},
			})
			return
		}
	}
	if strings.TrimSpace(document.Query) == "" {
		writeJSON(response, http.StatusBadRequest, map[string]any{
			"errors": []map[string]any{{"message": "query is required"}},
		})
		return
	}
	if graphQLIsMutation(document.Query) {
		if guard, ok := handler.principal.(cookieCSRFValidator); ok {
			if err := guard.ValidateCookieCSRF(request); err != nil {
				writeJSON(response, http.StatusForbidden, map[string]any{
					"errors": []map[string]any{{"message": "csrf token is invalid", "extensions": map[string]any{"code": "FORBIDDEN"}}},
				})
				return
			}
		}
	}
	principal := authz.Anonymous()
	if handler.principal != nil {
		resolved, err := handler.principal.Resolve(request)
		if err != nil {
			writeJSON(response, http.StatusUnauthorized, map[string]any{
				"errors": []map[string]any{{"message": "authentication failed"}},
			})
			return
		}
		principal = resolved
	}
	ctx := context.WithValue(request.Context(), principalContextKey{}, principal)
	result := graphql.Do(graphql.Params{
		Schema: handler.schema, RequestString: document.Query,
		VariableValues: document.Variables, OperationName: document.OperationName, Context: ctx,
	})
	writeJSON(response, http.StatusOK, result)
}

type graphQLRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

type principalContextKey struct{}

func principalFrom(ctx context.Context) authz.Principal {
	principal, _ := ctx.Value(principalContextKey{}).(authz.Principal)
	return principal
}

func (handler *Handler) buildSchema(manifest domainschema.Manifest) (graphql.Schema, error) {
	if err := manifest.Validate(); err != nil {
		return graphql.Schema{}, err
	}
	jsonScalar := newJSONScalar()
	queryFields := graphql.Fields{}
	mutationFields := graphql.Fields{}
	digest, _ := persist.ManifestDigest(manifest)
	identity := graphql.NewObject(graphql.ObjectConfig{
		Name: "SchemaIdentity",
		Fields: graphql.Fields{
			"name":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"version": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"digest":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	queryFields["schemaIdentity"] = &graphql.Field{
		Type: graphql.NewNonNull(identity),
		Resolve: func(graphql.ResolveParams) (any, error) {
			return map[string]any{"name": manifest.Name, "version": manifest.Version, "digest": digest}, nil
		},
	}
	formsetIssue := graphql.NewObject(graphql.ObjectConfig{
		Name: "FormsetIssue",
		Fields: graphql.Fields{
			"locale":  &graphql.Field{Type: graphql.String},
			"field":   &graphql.Field{Type: graphql.String},
			"code":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"message": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	formsetForm := graphql.NewObject(graphql.ObjectConfig{
		Name: "FormsetForm",
		Fields: graphql.Fields{
			"record":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"locales":  &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String)))},
			"fields":   &graphql.Field{Type: graphql.NewNonNull(jsonScalar)},
			"values":   &graphql.Field{Type: graphql.NewNonNull(jsonScalar)},
			"extra":    &graphql.Field{Type: jsonScalar},
			"issues":   &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(formsetIssue)))},
			"schema":   &graphql.Field{Type: graphql.NewNonNull(jsonScalar)},
			"payloads": &graphql.Field{Type: graphql.NewNonNull(jsonScalar)},
		},
	})
	queryFields["formsetSchema"] = &graphql.Field{
		Type: graphql.NewNonNull(formsetForm),
		Args: graphql.FieldConfigArgument{
			"resource": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
		Resolve: handler.formsetSchemaResolver(manifest),
	}
	queryFields["formset"] = &graphql.Field{
		Type: graphql.NewNonNull(formsetForm),
		Args: graphql.FieldConfigArgument{
			"resource": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			"id":       &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
		Resolve: handler.formsetResolver(manifest),
	}
	for _, resource := range manifest.Resources {
		resource := resource
		object := resourceObject(resource, jsonScalar)
		input := resourceInput(resource, jsonScalar)
		page := resourcePage(resource, object)
		listName := graphQLField(resource.Collection)
		itemName := graphQLField(resource.ID)
		queryFields[listName] = &graphql.Field{
			Type: page,
			Args: graphql.FieldConfigArgument{
				"page":       &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 1},
				"perPage":    &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 20},
				"search":     &graphql.ArgumentConfig{Type: graphql.String},
				"status":     &graphql.ArgumentConfig{Type: graphql.String},
				"visibility": &graphql.ArgumentConfig{Type: graphql.String},
				"locale":     &graphql.ArgumentConfig{Type: graphql.String},
			},
			Resolve: handler.listResolver(resource),
		}
		if itemName == listName {
			itemName += "ById"
		}
		queryFields[itemName] = &graphql.Field{
			Type: object,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			},
			Resolve: handler.getResolver(resource),
		}
		name := graphQLName(resource.ID)
		mutationFields["create"+name] = &graphql.Field{
			Type: graphql.NewNonNull(object),
			Args: graphql.FieldConfigArgument{
				"input":          &graphql.ArgumentConfig{Type: graphql.NewNonNull(input)},
				"idempotencyKey": &graphql.ArgumentConfig{Type: graphql.String},
			},
			Resolve: handler.createResolver(resource),
		}
		mutationFields["update"+name] = &graphql.Field{
			Type: graphql.NewNonNull(object),
			Args: graphql.FieldConfigArgument{
				"id":              &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				"input":           &graphql.ArgumentConfig{Type: graphql.NewNonNull(input)},
				"expectedVersion": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			},
			Resolve: handler.updateResolver(resource),
		}
		mutationFields["delete"+name] = &graphql.Field{
			Type: graphql.NewNonNull(graphql.Boolean),
			Args: graphql.FieldConfigArgument{
				"id":              &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				"expectedVersion": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			},
			Resolve: handler.deleteResolver(resource),
		}
	}
	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: queryFields}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: mutationFields}),
	})
}

func (handler *Handler) formsetSchemaResolver(manifest domainschema.Manifest) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		resource, ok := manifest.Resource(fmt.Sprint(params.Args["resource"]))
		if !ok {
			return nil, errors.New("resource does not exist")
		}
		form, err := forms.Bind(resource, forms.DocumentsFromEntry(domaincontent.Entry{}))
		if err != nil {
			return nil, err
		}
		return formsetView(resource, form)
	}
}

func (handler *Handler) formsetResolver(manifest domainschema.Manifest) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		resource, ok := manifest.Resource(fmt.Sprint(params.Args["resource"]))
		if !ok {
			return nil, errors.New("resource does not exist")
		}
		entry, err := handler.service.Get(params.Context, principalFrom(params.Context), domaincontent.ID(fmt.Sprint(params.Args["id"])))
		if err != nil {
			return nil, err
		}
		if entry.Kind != domaincontent.Kind(resource.ID) {
			return nil, errors.New("content was not found")
		}
		form, err := forms.BindEntry(resource, entry)
		if err != nil {
			return nil, err
		}
		return formsetView(resource, form)
	}
}

func formsetView(resource domainschema.Resource, form formset.Form) (map[string]any, error) {
	jsonSchema, err := forms.Schema(resource)
	if err != nil {
		return nil, err
	}
	issues := form.Issues
	if issues == nil {
		issues = []formset.Issue{}
	}
	return map[string]any{
		"record":   string(form.Record),
		"locales":  form.Locales,
		"fields":   form.Fields,
		"values":   form.Values,
		"extra":    form.Extra,
		"issues":   issues,
		"schema":   jsonSchema,
		"payloads": form.PayloadDocuments(),
	}, nil
}

func (handler *Handler) listResolver(resource domainschema.Resource) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		query := application.Query{
			Kinds: []domaincontent.Kind{domaincontent.Kind(resource.ID)},
			Page:  integerArgument(params.Args, "page", 1), PerPage: integerArgument(params.Args, "perPage", 20),
			Search: stringArgument(params.Args, "search"), Locale: stringArgument(params.Args, "locale"),
		}
		if status := stringArgument(params.Args, "status"); status != "" {
			query.Statuses = []domaincontent.Status{domaincontent.Status(status)}
		}
		result, err := handler.service.List(params.Context, principalFrom(params.Context), query)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(result.Entries))
		for _, entry := range result.Entries {
			if visibility := stringArgument(params.Args, "visibility"); visibility == "" || string(entry.Visibility) == visibility {
				items = append(items, graphQLRecord(entry))
			}
		}
		return map[string]any{
			"items": items, "page": result.Page.Number, "perPage": result.Page.PerPage,
			"total": result.Page.Total, "totalPages": result.Page.TotalPages,
		}, nil
	}
}

func (handler *Handler) getResolver(resource domainschema.Resource) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		entry, err := handler.service.Get(
			params.Context, principalFrom(params.Context), domaincontent.ID(fmt.Sprint(params.Args["id"])),
		)
		if err != nil || entry.Kind != domaincontent.Kind(resource.ID) {
			return nil, err
		}
		return graphQLRecord(entry), nil
	}
}

func (handler *Handler) createResolver(resource domainschema.Resource) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		input, _ := params.Args["input"].(map[string]any)
		entry, err := entryFromGraphQL(resource, input)
		if err != nil {
			return nil, err
		}
		principal := principalFrom(params.Context)
		idempotencyKey := strings.TrimSpace(fmt.Sprint(params.Args["idempotencyKey"]))
		if idempotencyKey != "" && idempotencyKey != "<nil>" {
			entry.ID = domaincontent.ID(uuid.NewSHA1(
				uuid.MustParse("33fb6db5-04e8-4a10-a650-fc22ddf7dc4e"),
				[]byte(principal.ID+"\x00"+resource.ID+"\x00"+idempotencyKey),
			).String())
			if existing, existingErr := handler.service.Get(params.Context, principal, entry.ID); existingErr == nil {
				return graphQLRecord(existing), nil
			}
		}
		created, err := handler.service.Create(params.Context, principal, entry)
		if err != nil {
			if idempotencyKey != "" && idempotencyKey != "<nil>" {
				if existing, existingErr := handler.service.Get(params.Context, principal, entry.ID); existingErr == nil {
					return graphQLRecord(existing), nil
				}
			}
			return nil, err
		}
		return graphQLRecord(created), nil
	}
}

func (handler *Handler) updateResolver(resource domainschema.Resource) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		principal := principalFrom(params.Context)
		id := domaincontent.ID(fmt.Sprint(params.Args["id"]))
		current, err := handler.service.Get(params.Context, principal, id)
		if err != nil {
			return nil, err
		}
		input, _ := params.Args["input"].(map[string]any)
		applyGraphQLInput(&current, input)
		updated, err := handler.service.Update(
			params.Context, principal, current, uint64(integerArgument(params.Args, "expectedVersion", 0)), "GraphQL update",
		)
		if err != nil {
			return nil, err
		}
		return graphQLRecord(updated), nil
	}
}

func (handler *Handler) deleteResolver(resource domainschema.Resource) graphql.FieldResolveFn {
	return func(params graphql.ResolveParams) (any, error) {
		principal := principalFrom(params.Context)
		entry, err := handler.service.Get(
			params.Context, principal, domaincontent.ID(fmt.Sprint(params.Args["id"])),
		)
		if err != nil {
			return false, err
		}
		if entry.Kind != domaincontent.Kind(resource.ID) {
			return false, errors.New("resource does not exist")
		}
		now := time.Now().UTC()
		entry.Status = domaincontent.StatusTrashed
		entry.DeletedAt = &now
		_, err = handler.service.Update(
			params.Context, principal, entry,
			uint64(integerArgument(params.Args, "expectedVersion", 0)), "GraphQL delete",
		)
		return err == nil, err
	}

}

func resourceObject(resource domainschema.Resource, jsonScalar *graphql.Scalar) *graphql.Object {
	fields := graphql.Fields{
		"id":               &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
		"version":          &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		"title":            &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"slug":             &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"titleLocalized":   &graphql.Field{Type: jsonScalar},
		"slugLocalized":    &graphql.Field{Type: jsonScalar},
		"contentLocalized": &graphql.Field{Type: jsonScalar},
		"excerptLocalized": &graphql.Field{Type: jsonScalar},
		"status":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"visibility":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"payloadRu":        &graphql.Field{Type: jsonScalar},
		"payloadEn":        &graphql.Field{Type: jsonScalar},
		"createdAt":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		"updatedAt":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
	}
	for _, field := range resource.Fields {
		if _, reserved := fields[graphQLField(field.ID)]; reserved || field.Sensitive {
			continue
		}
		fields[graphQLField(field.ID)] = &graphql.Field{Type: outputType(field, jsonScalar)}
	}
	return graphql.NewObject(graphql.ObjectConfig{Name: graphQLName(resource.ID), Fields: fields})
}

func resourceInput(resource domainschema.Resource, jsonScalar *graphql.Scalar) *graphql.InputObject {
	fields := graphql.InputObjectConfigFieldMap{
		"title":            &graphql.InputObjectFieldConfig{Type: graphql.String},
		"slug":             &graphql.InputObjectFieldConfig{Type: graphql.String},
		"titleLocalized":   &graphql.InputObjectFieldConfig{Type: jsonScalar},
		"slugLocalized":    &graphql.InputObjectFieldConfig{Type: jsonScalar},
		"contentLocalized": &graphql.InputObjectFieldConfig{Type: jsonScalar},
		"excerptLocalized": &graphql.InputObjectFieldConfig{Type: jsonScalar},
		"status":           &graphql.InputObjectFieldConfig{Type: graphql.String},
		"visibility":       &graphql.InputObjectFieldConfig{Type: graphql.String},
		"payloadRu":        &graphql.InputObjectFieldConfig{Type: jsonScalar},
		"payloadEn":        &graphql.InputObjectFieldConfig{Type: jsonScalar},
	}
	for _, field := range resource.Fields {
		name := graphQLField(field.ID)
		if _, reserved := fields[name]; reserved || field.ReadOnly {
			continue
		}
		fieldType := inputType(field, jsonScalar)
		if field.Required && !field.Nullable {
			fieldType = graphql.NewNonNull(fieldType)
		}
		fields[name] = &graphql.InputObjectFieldConfig{Type: fieldType}
	}
	return graphql.NewInputObject(graphql.InputObjectConfig{Name: graphQLName(resource.ID) + "Input", Fields: fields})
}

func resourcePage(resource domainschema.Resource, object *graphql.Object) *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: graphQLName(resource.ID) + "Page",
		Fields: graphql.Fields{
			"items":      &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(object)))},
			"page":       &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"perPage":    &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"total":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"totalPages": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})
}

func entryFromGraphQL(resource domainschema.Resource, input map[string]any) (domaincontent.Entry, error) {
	entry := domaincontent.Entry{
		Kind: domaincontent.Kind(resource.ID), Slug: domaincontent.LocalizedText{},
		Title: domaincontent.LocalizedText{}, Content: domaincontent.LocalizedText{},
		Excerpt: domaincontent.LocalizedText{}, Metadata: map[string]domaincontent.MetadataValue{},
	}
	applyGraphQLInput(&entry, input)
	return entry, nil
}

func applyGraphQLInput(entry *domaincontent.Entry, input map[string]any) {
	for key, value := range input {
		switch key {
		case "title":
			entry.Title["en"] = fmt.Sprint(value)
		case "slug":
			entry.Slug["en"] = fmt.Sprint(value)
		case "titleLocalized":
			applyLocalizedText(entry.Title, value)
		case "slugLocalized":
			applyLocalizedText(entry.Slug, value)
		case "contentLocalized":
			applyLocalizedText(entry.Content, value)
		case "excerptLocalized":
			applyLocalizedText(entry.Excerpt, value)
		case "status":
			entry.Status = domaincontent.Status(fmt.Sprint(value))
		case "visibility":
			entry.Visibility = domaincontent.Visibility(fmt.Sprint(value))
		case "payloadRu":
			entry.Metadata["payload_ru"] = domaincontent.MetadataValue{Value: value}
		case "payloadEn":
			entry.Metadata["payload_en"] = domaincontent.MetadataValue{Value: value}
		default:
			entry.Metadata[key] = domaincontent.MetadataValue{Value: value}
		}
	}
}

func applyLocalizedText(target domaincontent.LocalizedText, value any) {
	switch values := value.(type) {
	case map[string]any:
		for locale, localized := range values {
			target[locale] = fmt.Sprint(localized)
		}
	case map[string]string:
		for locale, localized := range values {
			target[locale] = localized
		}
	}
}

func graphQLRecord(entry domaincontent.Entry) map[string]any {
	record := map[string]any{
		"id": entry.ID, "version": entry.Version, "title": entry.Title.Value("en", "ru"),
		"slug": entry.Slug.Value("en", "ru"), "status": entry.Status, "visibility": entry.Visibility,
		"titleLocalized": entry.Title, "slugLocalized": entry.Slug,
		"contentLocalized": entry.Content, "excerptLocalized": entry.Excerpt,
		"createdAt": entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt": entry.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	for key, value := range entry.Metadata {
		switch key {
		case "payload_ru":
			record["payloadRu"] = value.Value
		case "payload_en":
			record["payloadEn"] = value.Value
		default:
			record[graphQLField(key)] = value.Value
		}
	}
	return record
}

func outputType(field domainschema.Field, jsonScalar *graphql.Scalar) graphql.Output {
	if field.Localized || field.Type == domainschema.FieldJSON || field.Type == domainschema.FieldCollection {
		return jsonScalar
	}
	var resolved graphql.Output
	switch field.Type {
	case domainschema.FieldBoolean:
		resolved = graphql.Boolean
	case domainschema.FieldInteger:
		resolved = graphql.Int
	case domainschema.FieldNumber, domainschema.FieldDecimal, domainschema.FieldMoney:
		resolved = graphql.Float
	case domainschema.FieldUUID, domainschema.FieldRelation, domainschema.FieldMedia:
		resolved = graphql.ID
	default:
		resolved = graphql.String
	}
	if field.Relation != nil && field.Relation.Cardinality == domainschema.CardinalityMany {
		return graphql.NewList(resolved)
	}
	return resolved
}

func inputType(field domainschema.Field, jsonScalar *graphql.Scalar) graphql.Input {
	if field.Localized || field.Type == domainschema.FieldJSON || field.Type == domainschema.FieldCollection {
		return jsonScalar
	}
	var resolved graphql.Input
	switch field.Type {
	case domainschema.FieldBoolean:
		resolved = graphql.Boolean
	case domainschema.FieldInteger:
		resolved = graphql.Int
	case domainschema.FieldNumber, domainschema.FieldDecimal, domainschema.FieldMoney:
		resolved = graphql.Float
	case domainschema.FieldUUID, domainschema.FieldRelation, domainschema.FieldMedia:
		resolved = graphql.ID
	default:
		resolved = graphql.String
	}
	if field.Relation != nil && field.Relation.Cardinality == domainschema.CardinalityMany {
		return graphql.NewList(resolved)
	}
	return resolved
}

func newJSONScalar() *graphql.Scalar {
	return graphql.NewScalar(graphql.ScalarConfig{
		Name: "JSON",
		Serialize: func(value any) any {
			return value
		},
		ParseValue: func(value any) any {
			return value
		},
		ParseLiteral: parseLiteral,
	})
}

func parseLiteral(value ast.Value) any {
	switch value := value.(type) {
	case *ast.StringValue:
		return value.Value
	case *ast.IntValue:
		resolved, _ := strconv.Atoi(value.Value)
		return resolved
	case *ast.FloatValue:
		resolved, _ := strconv.ParseFloat(value.Value, 64)
		return resolved
	case *ast.BooleanValue:
		return value.Value
	case *ast.ListValue:
		result := make([]any, 0, len(value.Values))
		for _, item := range value.Values {
			result = append(result, parseLiteral(item))
		}
		return result
	case *ast.ObjectValue:
		result := make(map[string]any, len(value.Fields))
		for _, field := range value.Fields {
			result[field.Name.Value] = parseLiteral(field.Value)
		}
		return result
	default:
		return nil
	}
}

func integerArgument(arguments map[string]any, key string, fallback int) int {
	value, exists := arguments[key]
	if !exists {
		return fallback
	}
	resolved, ok := value.(int)
	if !ok {
		return fallback
	}
	return resolved
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func graphQLIsMutation(query string) bool {
	trimmed := strings.TrimSpace(query)
	for strings.HasPrefix(trimmed, "#") {
		_, rest, found := strings.Cut(trimmed, "\n")
		if !found {
			return false
		}
		trimmed = strings.TrimSpace(rest)
	}
	return strings.HasPrefix(strings.ToLower(trimmed), "mutation")
}

func graphQLName(identifier string) string {
	parts := strings.FieldsFunc(identifier, func(character rune) bool {
		return character == '_' || character == '-'
	})
	for index, part := range parts {
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func graphQLField(identifier string) string {
	name := graphQLName(identifier)
	return strings.ToLower(name[:1]) + name[1:]
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
