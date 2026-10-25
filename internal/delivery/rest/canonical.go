package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/framework/pkg/core"
)

const (
	codexV1Timeout   = 10 * time.Second
	codexDefaultPage = 1
	codexDefaultSize = 20
	codexMaxPerPage  = 100
	// CodexV04Version is ui8kit/contract CODEX_V04_VERSION (the spec), not the
	// github.com/fastygo/codex module tag.
	CodexV04Version = "0.4.0"
	// CodexDeliveryWPKey is the Codex delivery-map key for the WordPress
	// adapter. The previous key was "wordpress".
	CodexDeliveryWPKey = "wp"
)

var canonicalQueryKeys = map[string]struct{}{
	"locale":   {},
	"page":     {},
	"per_page": {},
}

type canonicalListDocument struct {
	Data       []json.RawMessage  `json:"data"`
	Pagination paginationDocument `json:"pagination"`
}

func (handler *CodexHandler) canonicalDiscovery(response http.ResponseWriter, request *http.Request) {
	if err := rejectUnknownQuery(request); err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	digest, err := handler.manifestDigest()
	if err != nil {
		writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "manifest digest failed", err))
		return
	}
	writeCanonicalJCS(response, http.StatusOK, map[string]any{
		"codex_version":   CodexV04Version,
		"manifest_digest": digest,
		"profile":         canonical.EntryProfile,
	})
}

func (handler *CodexHandler) canonicalList(response http.ResponseWriter, request *http.Request) {
	handler.serveCanonicalList(response, request, domaincontent.Kind(request.PathValue("kind")), true)
}

func (handler *CodexHandler) canonicalSettings(response http.ResponseWriter, request *http.Request) {
	handler.serveCanonicalSettings(response, request, "")
}

func (handler *CodexHandler) canonicalSettingsGet(response http.ResponseWriter, request *http.Request) {
	handler.serveCanonicalSettings(response, request, strings.TrimSpace(request.PathValue("id")))
}

func (handler *CodexHandler) serveCanonicalList(
	response http.ResponseWriter,
	request *http.Request,
	kind domaincontent.Kind,
	requireKnownKind bool,
) {
	ctx, cancel := context.WithTimeout(request.Context(), codexV1Timeout)
	defer cancel()
	query, err := handler.parseCanonicalQuery(request)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	if !handler.localeAllowed(query.locale, query.defaultLocale) {
		writeCanonicalError(response, request, canonicalNotFound("locale was not found"))
		return
	}
	if requireKnownKind && !handler.knownKind(kind) {
		writeCanonicalError(response, request, canonicalNotFound("kind was not found"))
		return
	}
	entries, err := handler.loadPublicKind(ctx, kind)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	items := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		projected, ok := contentcompat.ProjectExact(entry, query.locale)
		if !ok {
			continue
		}
		body, _, err := canonicalizeEntry(projected)
		if err != nil {
			writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "canonical encoding failed", err))
			return
		}
		items = append(items, json.RawMessage(body))
	}
	sort.SliceStable(items, func(i, j int) bool {
		leftID, leftLocale := canonicalKeys(items[i])
		rightID, rightLocale := canonicalKeys(items[j])
		if leftID != rightID {
			return leftID < rightID
		}
		return leftLocale < rightLocale
	})
	writeCanonicalJCS(response, http.StatusOK, paginateCanonical(items, query.page, query.perPage))
}

func (handler *CodexHandler) canonicalGet(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), codexV1Timeout)
	defer cancel()
	query, err := handler.parseCanonicalQuery(request)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	if !handler.localeAllowed(query.locale, query.defaultLocale) {
		writeCanonicalError(response, request, canonicalNotFound("locale was not found"))
		return
	}
	kind := domaincontent.Kind(request.PathValue("kind"))
	if !handler.knownKind(kind) {
		writeCanonicalError(response, request, canonicalNotFound("kind was not found"))
		return
	}
	entry, err := handler.content.Get(ctx, canonicalReader(), domaincontent.ID(request.PathValue("id")))
	if err != nil || entry.Kind != kind || !entry.IsPublicAt(time.Now().UTC()) {
		writeCanonicalError(response, request, canonicalNotFound("content was not found"))
		return
	}
	projected, ok := contentcompat.ProjectExact(entry, query.locale)
	if !ok {
		writeCanonicalError(response, request, canonicalNotFound("content was not found"))
		return
	}
	body, digest, err := canonicalizeEntry(projected)
	if err != nil {
		writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "canonical encoding failed", err))
		return
	}
	writeCanonicalBytes(response, http.StatusOK, body, digest)
}

func (handler *CodexHandler) canonicalTaxonomies(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), codexV1Timeout)
	defer cancel()
	query, err := handler.parseCanonicalQuery(request)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	if !handler.localeAllowed(query.locale, query.defaultLocale) {
		writeCanonicalError(response, request, canonicalNotFound("locale was not found"))
		return
	}
	items, err := handler.taxonomies.ListDefinitions(ctx, authz.Anonymous())
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		return items[i].Locale < items[j].Locale
	})
	encoded := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if query.locale != "" && item.Locale != "" && item.Locale != query.locale {
			continue
		}
		body, err := canonicalizeTaxonomy(item)
		if err != nil {
			writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "taxonomy encoding failed", err))
			return
		}
		encoded = append(encoded, body)
	}
	writeCanonicalJCS(response, http.StatusOK, paginateCanonical(encoded, query.page, query.perPage))
}

func (handler *CodexHandler) canonicalTerms(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), codexV1Timeout)
	defer cancel()
	query, err := handler.parseCanonicalQuery(request)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	if !handler.localeAllowed(query.locale, query.defaultLocale) {
		writeCanonicalError(response, request, canonicalNotFound("locale was not found"))
		return
	}
	taxonomyID := strings.TrimSpace(request.PathValue("taxonomy_id"))
	items, err := handler.taxonomies.ListTerms(ctx, authz.Anonymous(), taxonomyID)
	if err != nil {
		writeCanonicalError(response, request, canonicalNotFound("taxonomy was not found"))
		return
	}
	encoded := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if query.locale != "" && item.Locale != "" && item.Locale != query.locale {
			continue
		}
		body, err := canonicalizeTerm(item)
		if err != nil {
			writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "canonical term encoding failed", err))
			return
		}
		encoded = append(encoded, json.RawMessage(body))
	}
	sort.SliceStable(encoded, func(i, j int) bool {
		leftID, leftLocale := canonicalKeys(encoded[i])
		rightID, rightLocale := canonicalKeys(encoded[j])
		if leftID != rightID {
			return leftID < rightID
		}
		return leftLocale < rightLocale
	})
	writeCanonicalJCS(response, http.StatusOK, paginateCanonical(encoded, query.page, query.perPage))
}

func (handler *CodexHandler) serveCanonicalSettings(response http.ResponseWriter, request *http.Request, id string) {
	ctx, cancel := context.WithTimeout(request.Context(), codexV1Timeout)
	defer cancel()
	query, err := handler.parseCanonicalQuery(request)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	if !handler.localeAllowed(query.locale, query.defaultLocale) {
		writeCanonicalError(response, request, canonicalNotFound("locale was not found"))
		return
	}
	items, err := handler.loadCanonicalSettings(ctx)
	if err != nil {
		writeCanonicalError(response, request, err)
		return
	}
	matched := make([]domaincontent.Settings, 0, len(items))
	for _, item := range items {
		if id != "" && string(item.ID) != id {
			continue
		}
		if query.locale != "" && item.Locale != query.locale {
			continue
		}
		matched = append(matched, item)
	}
	if id != "" {
		if len(matched) == 0 {
			writeCanonicalError(response, request, canonicalNotFound("settings were not found"))
			return
		}
		body, digest, err := canonicalizeSettings(matched[0])
		if err != nil {
			writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "canonical settings encoding failed", err))
			return
		}
		writeCanonicalBytes(response, http.StatusOK, body, digest)
		return
	}
	encoded := make([]json.RawMessage, 0, len(matched))
	for _, item := range matched {
		body, _, err := canonicalizeSettings(item)
		if err != nil {
			writeCanonicalError(response, request, core.WrapDomainError(core.ErrorCodeInternal, "canonical settings encoding failed", err))
			return
		}
		encoded = append(encoded, body)
	}
	writeCanonicalJCS(response, http.StatusOK, paginateCanonical(encoded, query.page, query.perPage))
}

type canonicalQuery struct {
	locale        string
	defaultLocale string
	page          int
	perPage       int
}

func (handler *CodexHandler) parseCanonicalQuery(request *http.Request) (canonicalQuery, error) {
	if err := rejectUnknownQuery(request); err != nil {
		return canonicalQuery{}, err
	}
	defaultLocale := handler.siteDefaultLocale(request.Context())
	values := request.URL.Query()
	locale := contentcompat.NormalizeLocale(values.Get("locale"))
	if locale == "" {
		locale = defaultLocale
	} else if !domaincontent.ValidLocale(locale) {
		return canonicalQuery{}, canonicalValidation("locale is invalid", map[string]any{"locale": locale})
	}
	page, err := canonicalPositive(values.Get("page"), codexDefaultPage, "page")
	if err != nil {
		return canonicalQuery{}, err
	}
	perPage, err := canonicalPositive(values.Get("per_page"), codexDefaultSize, "per_page")
	if err != nil {
		return canonicalQuery{}, err
	}
	if perPage > codexMaxPerPage {
		perPage = codexMaxPerPage
	}
	return canonicalQuery{locale: locale, defaultLocale: defaultLocale, page: page, perPage: perPage}, nil
}

func rejectUnknownQuery(request *http.Request) error {
	for key := range request.URL.Query() {
		if _, ok := canonicalQueryKeys[key]; !ok {
			return canonicalValidation("unknown query parameter", map[string]any{"parameter": key})
		}
	}
	return nil
}

func canonicalPositive(raw string, fallback int, field string) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, canonicalValidation(field+" must be a positive integer", map[string]any{field: raw})
	}
	return value, nil
}

func (handler *CodexHandler) knownKind(kind domaincontent.Kind) bool {
	switch kind {
	case "media", "user", domaincontent.KindPost, domaincontent.KindPage, domaincontent.KindMenu, domaincontent.KindSetting:
		return true
	}
	_, ok := handler.manifest.Resource(string(kind))
	return ok
}

func (handler *CodexHandler) localeAllowed(locale, defaultLocale string) bool {
	locale = contentcompat.NormalizeLocale(locale)
	if locale == "" || locale == defaultLocale {
		return true
	}
	for _, item := range handler.availableLocales {
		if item == locale {
			return true
		}
	}
	return false
}

// siteDefaultLocale is the one /codex/v1 locale resolver: settings.language, else "en".
func (handler *CodexHandler) siteDefaultLocale(ctx context.Context) string {
	items, err := handler.loadCanonicalSettings(ctx)
	if err == nil {
		for _, item := range items {
			language := contentcompat.NormalizeLocale(item.Language)
			if domaincontent.ValidLocale(language) {
				return language
			}
		}
	}
	return "en"
}

func (handler *CodexHandler) loadCanonicalSettings(ctx context.Context) ([]domaincontent.Settings, error) {
	entries, err := handler.loadPublicKind(ctx, domaincontent.KindSetting)
	if err != nil {
		return nil, err
	}
	items := make([]domaincontent.Settings, 0, len(entries))
	for _, entry := range entries {
		items = append(items, settingsFromEntry(contentcompat.PublicCodex(entry)))
	}
	sortSettings(items)
	return items, nil
}

func (handler *CodexHandler) loadPublicKind(ctx context.Context, kind domaincontent.Kind) ([]domaincontent.Entry, error) {
	now := time.Now().UTC()
	var entries []domaincontent.Entry
	for page := 1; ; page++ {
		result, err := handler.content.List(ctx, canonicalReader(), application.Query{
			Kinds: []domaincontent.Kind{kind}, Page: page, PerPage: 100, Sort: "id",
			PublicOnly: true, PublicAt: now,
		})
		if err != nil {
			return nil, err
		}
		entries = append(entries, result.Entries...)
		if result.Page.TotalPages == 0 || page >= result.Page.TotalPages {
			break
		}
	}
	return entries, nil
}

func canonicalizeEntry(entry domaincontent.Entry) ([]byte, string, error) {
	projected := contentcompat.PublicCodex(entry)
	projected = projectCanonicalMedia(projected)
	raw, err := json.Marshal(projected)
	if err != nil {
		return nil, "", err
	}
	text, err := canonical.Canonicalize(string(raw))
	if err != nil {
		return nil, "", err
	}
	digest, err := canonical.Digest(string(raw))
	if err != nil {
		return nil, "", err
	}
	return []byte(text), digest, nil
}

// projectCanonicalMedia maps WordPress-style metadata.post onto the v0.4
// attached_to field. /go-json/go/v2 keeps emitting post unchanged.
func projectCanonicalMedia(entry domaincontent.Entry) domaincontent.Entry {
	if entry.Kind != "media" {
		return entry
	}
	post, ok := entry.Metadata["post"]
	if !ok {
		return entry
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]domaincontent.MetadataValue{}
	} else {
		cloned := make(map[string]domaincontent.MetadataValue, len(entry.Metadata))
		for key, value := range entry.Metadata {
			cloned[key] = value
		}
		entry.Metadata = cloned
	}
	delete(entry.Metadata, "post")
	entry.Metadata["attached_to"] = post
	if len(entry.Metadata) == 0 {
		entry.Metadata = nil
	}
	return entry
}

func paginateCanonical(items []json.RawMessage, page, perPage int) canonicalListDocument {
	total := len(items)
	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	data := items[start:end]
	if data == nil {
		data = []json.RawMessage{}
	}
	return canonicalListDocument{
		Data: data,
		Pagination: paginationDocument{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	}
}

func canonicalKeys(raw json.RawMessage) (string, string) {
	var document struct {
		ID     string `json:"id"`
		Locale string `json:"locale"`
	}
	_ = json.Unmarshal(raw, &document)
	return document.ID, document.Locale
}

func canonicalReader() authz.Principal {
	return authz.NewPrincipal("codex-v1", authz.CapabilityContentReadPrivate)
}

func canonicalNotFound(message string) error {
	return core.NewDomainError(core.ErrorCodeNotFound, message)
}

func canonicalValidation(message string, details map[string]any) error {
	return canonicalCodedError{code: "validation_error", message: message, status: http.StatusBadRequest, details: details}
}

type canonicalCodedError struct {
	code    string
	message string
	status  int
	details map[string]any
}

func (err canonicalCodedError) Error() string {
	return err.message
}

func writeCanonicalBytes(response http.ResponseWriter, status int, body []byte, digest string) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	if digest != "" {
		response.Header().Set("Codex-Digest", digest)
		if hexID := digestHex(digest); hexID != "" {
			response.Header().Set("ETag", `"`+hexID+`"`)
		}
	}
	response.WriteHeader(status)
	_, _ = response.Write(body)
}

func writeCanonicalError(response http.ResponseWriter, request *http.Request, err error) {
	code := "internal_error"
	message := "internal server error"
	status := http.StatusInternalServerError
	var details map[string]any
	var coded canonicalCodedError
	if errors.As(err, &coded) {
		code, message, status, details = coded.code, coded.message, coded.status, coded.details
	} else {
		var domain core.DomainError
		if errors.As(err, &domain) {
			status = domain.StatusCode()
			message = domain.Message
			code = canonicalErrorCode(string(domain.Code), status)
			if domain.Code == core.ErrorCodeInternal {
				message = "internal server error"
			}
		}
	}
	errorBody := map[string]any{
		"code":    code,
		"message": message,
		"status":  status,
	}
	if len(details) > 0 {
		errorBody["details"] = details
	}
	if id := requestID(request); id != "" {
		errorBody["request_id"] = id
	}
	writeCanonicalJCS(response, status, map[string]any{"error": errorBody})
}

func canonicalErrorCode(code string, status int) string {
	switch code {
	case "validation":
		return "validation_error"
	case "internal":
		return "internal_error"
	case "not_found", "unauthorized", "forbidden", "conflict":
		return code
	}
	switch status {
	case http.StatusBadRequest:
		return "validation_error"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	default:
		if code == "" {
			return "internal_error"
		}
		return code
	}
}

func digestHex(digest string) string {
	if index := strings.LastIndex(digest, ":"); index >= 0 && index+1 < len(digest) {
		return digest[index+1:]
	}
	return digest
}
