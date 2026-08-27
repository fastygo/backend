package rest

import (
	"errors"
	"net/http"
	"reflect"
	"strings"

	domaincontent "github.com/fastygo/backend/internal/domain/content"
)

const collectionPrefix = "/go-json/go/v2/"

type collectionCapability uint16

const (
	collectionList collectionCapability = 1 << iota
	collectionCreate
	collectionGet
	collectionUpdate
	collectionDelete
	collectionBySlug
	collectionTransition
	collectionRevisions
	collectionRestoreRevision
	collectionEntryForm
)

type collectionEndpoint struct {
	collection   string
	kind         domaincontent.Kind
	capabilities collectionCapability
}

func (endpoint collectionEndpoint) supports(capability collectionCapability) bool {
	return endpoint.capabilities&capability != 0
}

// CollectionRouter owns the grammar for manifest-backed Codex collections.
// It gives literal path segments precedence over dynamic IDs.
type CollectionRouter struct {
	codex       *CodexHandler
	content     *ContentHandler
	collections map[string]collectionEndpoint
}

func NewCollectionRouter(
	codex *CodexHandler,
	content *ContentHandler,
) (*CollectionRouter, error) {
	if codex == nil || content == nil {
		return nil, errors.New("codex and content handlers are required")
	}
	if !reflect.DeepEqual(codex.manifest, content.manifest) {
		return nil, errors.New("codex and content handlers must use the same manifest")
	}
	manifest := codex.manifest
	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	router := &CollectionRouter{
		codex:       codex,
		content:     content,
		collections: make(map[string]collectionEndpoint, len(manifest.Resources)+1),
	}
	router.add(collectionEndpoint{
		collection:   "media",
		kind:         "media",
		capabilities: collectionList | collectionGet | collectionUpdate | collectionDelete,
	})
	for _, resource := range manifest.Resources {
		if !resource.RegistersCodexCollection() {
			continue
		}
		router.add(collectionEndpoint{
			collection: resource.Collection,
			kind:       domaincontent.Kind(resource.ID),
			capabilities: collectionList | collectionCreate | collectionGet | collectionUpdate |
				collectionDelete | collectionBySlug | collectionTransition | collectionRevisions |
				collectionRestoreRevision | collectionEntryForm,
		})
	}
	return router, nil
}

func (router *CollectionRouter) add(endpoint collectionEndpoint) {
	router.collections[endpoint.collection] = endpoint
}

func (router *CollectionRouter) Routes(mux *http.ServeMux) {
	mux.Handle(collectionPrefix, router)
}

func (router *CollectionRouter) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	path := strings.TrimPrefix(request.URL.Path, collectionPrefix)
	segments := strings.Split(path, "/")
	if path == "" || len(segments) == 0 || segments[0] == "" {
		http.NotFound(response, request)
		return
	}
	endpoint, found := router.collections[segments[0]]
	if !found {
		http.NotFound(response, request)
		return
	}

	var matched []collectionRoute
	var allowed = make(map[string]struct{})
	for _, route := range collectionRoutes {
		if !endpoint.supports(route.capability) {
			continue
		}
		if !route.matches(segments[1:]) {
			continue
		}
		allowed[route.method] = struct{}{}
		if request.Method == route.method {
			matched = append(matched, route)
		}
	}
	if len(matched) == 0 {
		if len(allowed) > 0 {
			writeMethodNotAllowed(response, allowed)
			return
		}
		http.NotFound(response, request)
		return
	}

	route := matched[0]
	for _, candidate := range matched[1:] {
		if candidate.specificity() > route.specificity() {
			route = candidate
		}
	}
	request.SetPathValue("collection", endpoint.collection)
	route.bind(request, segments[1:])
	route.serve(router, endpoint, response, request)
}

type collectionRoute struct {
	method     string
	segments   []string
	capability collectionCapability
	serve      func(*CollectionRouter, collectionEndpoint, http.ResponseWriter, *http.Request)
}

func (route collectionRoute) matches(segments []string) bool {
	if len(route.segments) != len(segments) {
		return false
	}
	for index, segment := range route.segments {
		if strings.HasPrefix(segment, ":") {
			continue
		}
		if segment != segments[index] {
			return false
		}
	}
	return true
}

func (route collectionRoute) specificity() int {
	score := 0
	for _, segment := range route.segments {
		score <<= 1
		if !strings.HasPrefix(segment, ":") {
			score++
		}
	}
	return score
}

func (route collectionRoute) bind(request *http.Request, segments []string) {
	for index, segment := range route.segments {
		if strings.HasPrefix(segment, ":") {
			request.SetPathValue(strings.TrimPrefix(segment, ":"), segments[index])
		}
	}
}

var collectionRoutes = []collectionRoute{
	{method: http.MethodGet, capability: collectionList, serve: serveList},
	{method: http.MethodPost, capability: collectionCreate, serve: serveCreate},
	{method: http.MethodGet, segments: []string{"by-slug", ":slug"}, capability: collectionBySlug, serve: serveBySlug},
	{method: http.MethodGet, segments: []string{":id"}, capability: collectionGet, serve: serveGet},
	{method: http.MethodPatch, segments: []string{":id"}, capability: collectionUpdate, serve: serveUpdate},
	{method: http.MethodDelete, segments: []string{":id"}, capability: collectionDelete, serve: serveDelete},
	{method: http.MethodPost, segments: []string{":id", "transitions"}, capability: collectionTransition, serve: serveTransition},
	{method: http.MethodGet, segments: []string{":id", "revisions"}, capability: collectionRevisions, serve: serveRevisions},
	{method: http.MethodPost, segments: []string{":id", "revisions", ":revision", "restore"}, capability: collectionRestoreRevision, serve: serveRestoreRevision},
	{method: http.MethodGet, segments: []string{":id", "form"}, capability: collectionEntryForm, serve: serveEntryForm},
}

func serveList(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.codex.list(endpoint.kind)(response, request)
}

func serveCreate(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.codex.create(endpoint.kind)(response, request)
}

func serveBySlug(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.codex.bySlug(endpoint.kind)(response, request)
}

func serveGet(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.codex.get(endpoint.kind)(response, request)
}

func serveUpdate(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.codex.update(endpoint.kind)(response, request)
}

func serveDelete(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.codex.trash(endpoint.kind)(response, request)
}

func serveTransition(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.content.transition(response, request)
}

func serveRevisions(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.content.revisions(response, request)
}

func serveRestoreRevision(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.content.restoreRevision(response, request)
}

func serveEntryForm(router *CollectionRouter, endpoint collectionEndpoint, response http.ResponseWriter, request *http.Request) {
	router.content.entryForm(response, request)
}

func writeMethodNotAllowed(response http.ResponseWriter, allowed map[string]struct{}) {
	methods := make([]string, 0, len(allowed))
	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete,
	} {
		if _, found := allowed[method]; found {
			methods = append(methods, method)
		}
	}
	response.Header().Set("Allow", strings.Join(methods, ", "))
	response.WriteHeader(http.StatusMethodNotAllowed)
}
