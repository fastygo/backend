package rest

import (
	"errors"
	"net/http"
	"strings"

	application "github.com/fastygo/backend/internal/application/content"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/authz"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/framework/pkg/core"
)

// CodexHandler exposes the stable go-codex Level 0/1 compatibility surface.
type CodexHandler struct {
	content    *application.Service
	taxonomies *applicationtaxonomy.Service
	principal  PrincipalResolver
	manifest   schema.Manifest
}

func NewCodexHandler(
	content *application.Service,
	taxonomies *applicationtaxonomy.Service,
	principal PrincipalResolver,
	manifest schema.Manifest,
) (*CodexHandler, error) {
	if content == nil || taxonomies == nil {
		return nil, errors.New("codex content and taxonomy services are required")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &CodexHandler{
		content: content, taxonomies: taxonomies, principal: principal, manifest: manifest,
	}, nil
}

func (handler *CodexHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", handler.origin)
	mux.HandleFunc("GET /go-json", handler.root)
	mux.HandleFunc("GET /go-json/go/v2/{$}", handler.discovery)
	mux.HandleFunc("GET /go-json/go/v2/content-types", handler.contentTypes)
	mux.HandleFunc("GET /go-json/go/v2/types", handler.contentTypes)
	mux.HandleFunc("GET /go-json/go/v2/taxonomies", handler.listTaxonomies)
	mux.HandleFunc("GET /go-json/go/v2/taxonomies/{taxonomy}", handler.listTerms)
	mux.HandleFunc("GET /go-json/go/v2/search", handler.search)
	mux.HandleFunc("GET /go-json/go/v2/menus", handler.list("menu"))
	mux.HandleFunc("GET /go-json/go/v2/menus/{slug}", handler.bySlug("menu"))
	mux.HandleFunc("GET /go-json/go/v2/settings", handler.list("setting"))
}

func (handler *CodexHandler) origin(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{
		"name":    handler.manifest.Name,
		"version": "2",
		"kind":    "codex",
		"routes": map[string]any{
			"json":    "/go-json",
			"go/v2":   "/go-json/go/v2/",
			"graphql": "/go-graphql",
			"healthz": "/healthz",
			"readyz":  "/readyz",
			"openapi": "/go-json/go/v2/openapi.json",
			"types":   "/go-json/go/v2/types",
		},
		"authentication": []string{"bearer"},
		"links": map[string]any{
			"self": "/", "json": "/go-json", "v2": "/go-json/go/v2/",
		},
	})
}

func (handler *CodexHandler) root(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{
		"name": handler.manifest.Name, "version": "2",
		"routes":         map[string]any{"go/v2": "/go-json/go/v2/"},
		"authentication": []string{"bearer"},
		"links":          map[string]any{"self": "/go-json"},
	})
}

func (handler *CodexHandler) discovery(response http.ResponseWriter, _ *http.Request) {
	routes := map[string]any{
		"posts": "/go-json/go/v2/posts", "pages": "/go-json/go/v2/pages",
		"media": "/go-json/go/v2/media", "taxonomies": "/go-json/go/v2/taxonomies",
		"menus": "/go-json/go/v2/menus", "settings": "/go-json/go/v2/settings",
		"search": "/go-json/go/v2/search", "contentTypes": "/go-json/go/v2/content-types",
		"types": "/go-json/go/v2/types",
	}
	for _, resource := range handler.manifest.Resources {
		if resource.RegistersCodexCollection() {
			routes[resource.Collection] = "/go-json/go/v2/" + resource.Collection
		}
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"name": handler.manifest.Name, "version": "2",
		"routes": routes, "authentication": []string{"bearer"},
		"links": map[string]any{"self": "/go-json/go/v2/"},
	})
}

func (handler *CodexHandler) list(kind domaincontent.Kind) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := handler.resolve(response, request)
		if !ok {
			return
		}
		query, err := parseQuery(request)
		if err != nil {
			writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid query", err))
			return
		}
		query.Kinds = []domaincontent.Kind{kind}
		result, err := handler.content.List(request.Context(), principal, query)
		if err != nil {
			writeError(response, request, err)
			return
		}
		if visibility := strings.TrimSpace(request.URL.Query().Get("visibility")); visibility != "" {
			matched := result.Entries[:0]
			for _, entry := range result.Entries {
				if string(entry.Visibility) == visibility {
					matched = append(matched, entry)
				}
			}
			result.Entries = matched
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"data": handler.codexEntries(result.Entries), "pagination": projectPage(result.Page),
		})
	}
}

func (handler *CodexHandler) get(kind domaincontent.Kind) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := handler.resolve(response, request)
		if !ok {
			return
		}
		entry, err := handler.content.Get(request.Context(), principal, domaincontent.ID(request.PathValue("id")))
		if err != nil || entry.Kind != kind {
			if err == nil {
				err = core.NewDomainError(core.ErrorCodeNotFound, "content was not found")
			}
			writeError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"data": handler.codexEntry(entry)})
	}
}

func (handler *CodexHandler) bySlug(kind domaincontent.Kind) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		principal, ok := handler.resolve(response, request)
		if !ok {
			return
		}
		locale := strings.TrimSpace(request.URL.Query().Get("locale"))
		if locale == "" {
			locale = "en"
		}
		entry, err := handler.content.GetBySlug(
			request.Context(), principal, kind, locale, request.PathValue("slug"),
		)
		if err != nil {
			writeError(response, request, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"data": handler.codexEntry(entry)})
	}
}

func (handler *CodexHandler) create(kind domaincontent.Kind) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if !handler.guardMutation(response, request) {
			return
		}
		principal, ok := handler.resolve(response, request)
		if !ok {
			return
		}
		var entry domaincontent.Entry
		if err := decodeEntryJSON(response, request, &entry); err != nil {
			writeError(response, request, err)
			return
		}
		entry.Kind = kind
		if entry.Status == "" {
			entry.Status = domaincontent.StatusDraft
		}
		if entry.Visibility == "" {
			entry.Visibility = domaincontent.VisibilityPublic
		}
		created, err := handler.content.Create(request.Context(), principal, entry)
		if err != nil {
			writeError(response, request, err)
			return
		}
		response.Header().Set("Location", request.URL.Path+"/"+string(created.ID))
		response.Header().Set("ETag", versionETag(created.Version))
		writeJSON(response, http.StatusCreated, map[string]any{"data": handler.codexEntry(created)})
	}
}

func (handler *CodexHandler) update(kind domaincontent.Kind) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if !handler.guardMutation(response, request) {
			return
		}
		principal, ok := handler.resolve(response, request)
		if !ok {
			return
		}
		current, err := handler.content.Get(
			request.Context(), principal, domaincontent.ID(request.PathValue("id")),
		)
		if err != nil || current.Kind != kind {
			writeError(response, request, core.NewDomainError(core.ErrorCodeNotFound, "content was not found"))
			return
		}
		var desired domaincontent.Entry
		if err := decodeEntryJSON(response, request, &desired); err != nil {
			writeError(response, request, err)
			return
		}
		mergeEntry(&current, desired)
		version, err := expectedVersion(request, desired.Version)
		if err != nil {
			writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
			return
		}
		updated, err := handler.content.Update(request.Context(), principal, current, version, "go-codex update")
		if err != nil {
			writeError(response, request, err)
			return
		}
		response.Header().Set("ETag", versionETag(updated.Version))
		writeJSON(response, http.StatusOK, map[string]any{"data": handler.codexEntry(updated)})
	}
}

func (handler *CodexHandler) trash(kind domaincontent.Kind) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if !handler.guardMutation(response, request) {
			return
		}
		principal, ok := handler.resolve(response, request)
		if !ok {
			return
		}
		version, err := expectedVersion(request, 0)
		if err != nil {
			writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
			return
		}
		_, err = handler.content.Transition(request.Context(), principal, domaincontent.ID(request.PathValue("id")), application.Transition{
			Kind: kind, Status: domaincontent.StatusTrashed, ExpectedVersion: version,
		})
		if err != nil {
			writeError(response, request, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}
}

func (handler *CodexHandler) contentTypes(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]any{"data": persist.TypesFromManifest(handler.manifest)})
}

func (handler *CodexHandler) listTaxonomies(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolve(response, request)
	if !ok {
		return
	}
	items, err := handler.taxonomies.ListDefinitions(request.Context(), principal)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": items})
}

func (handler *CodexHandler) listTerms(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolve(response, request)
	if !ok {
		return
	}
	items, err := handler.taxonomies.ListTerms(request.Context(), principal, request.PathValue("taxonomy"))
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": items})
}

func (handler *CodexHandler) search(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolve(response, request)
	if !ok {
		return
	}
	query, err := parseQuery(request)
	if err != nil {
		writeError(response, request, err)
		return
	}
	query.Search = strings.TrimSpace(request.URL.Query().Get("q"))
	for _, resource := range handler.manifest.Resources {
		if resource.Public {
			query.Kinds = append(query.Kinds, domaincontent.Kind(resource.ID))
		}
	}
	result, err := handler.content.List(request.Context(), principal, query)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"data": handler.codexEntries(result.Entries), "pagination": projectPage(result.Page),
	})
}

func (handler *CodexHandler) resolve(
	response http.ResponseWriter,
	request *http.Request,
) (authz.Principal, bool) {
	if handler.principal == nil {
		return authz.Anonymous(), true
	}
	principal, err := handler.principal.Resolve(request)
	if err != nil {
		writeError(response, request, err)
		return authz.Principal{}, false
	}
	return principal, true
}

func (handler *CodexHandler) codexEntries(entries []domaincontent.Entry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, handler.codexEntry(entry))
	}
	return items
}

func (handler *CodexHandler) collectionFor(kind domaincontent.Kind) string {
	if kind == "media" {
		return "media"
	}
	if resource, ok := handler.manifest.Resource(string(kind)); ok {
		return resource.Collection
	}
	return string(kind)
}

func (handler *CodexHandler) codexEntry(entry domaincontent.Entry) map[string]any {
	metadata := make(map[string]any, len(entry.Metadata))
	for key, value := range entry.Metadata {
		metadata[key] = value.Value
	}
	termIDs := make([]string, 0, len(entry.Terms))
	for _, reference := range entry.Terms {
		termIDs = append(termIDs, reference.TermID)
	}
	record := "/go-json/go/v2/" + handler.collectionFor(entry.Kind) + "/" + string(entry.ID)
	return map[string]any{
		"id": entry.ID, "kind": entry.Kind, "version": entry.Version,
		"status": entry.Status, "visibility": entry.Visibility,
		"slug": entry.Slug, "title": entry.Title, "content": entry.Content,
		"excerpt": entry.Excerpt, "author_id": entry.AuthorID,
		"featured_media_id": entry.FeaturedMediaID, "taxonomy_ids": termIDs,
		"metadata": metadata, "created_at": entry.CreatedAt, "updated_at": entry.UpdatedAt,
		"published_at": entry.PublishedAt,
		"links": map[string]string{
			"self": record, "form": record + "/form", "revisions": record + "/revisions",
		},
	}
}

func (handler *CodexHandler) guardMutation(response http.ResponseWriter, request *http.Request) bool {
	guard, ok := handler.principal.(interface{ ValidateCookieCSRF(*http.Request) error })
	if !ok {
		return true
	}
	if err := guard.ValidateCookieCSRF(request); err != nil {
		writeError(response, request, core.NewDomainError(core.ErrorCodeForbidden, "csrf token is invalid"))
		return false
	}
	return true
}

func mergeEntry(target *domaincontent.Entry, patch domaincontent.Entry) {
	if patch.Status != "" {
		target.Status = patch.Status
	}
	if patch.Visibility != "" {
		target.Visibility = patch.Visibility
	}
	if len(patch.Slug) > 0 {
		target.Slug = patch.Slug
	}
	if len(patch.Title) > 0 {
		target.Title = patch.Title
	}
	if len(patch.Content) > 0 {
		target.Content = patch.Content
	}
	if len(patch.Excerpt) > 0 {
		target.Excerpt = patch.Excerpt
	}
	if patch.Metadata != nil {
		target.Metadata = patch.Metadata
	}
	if patch.Terms != nil {
		target.Terms = patch.Terms
	}
}
