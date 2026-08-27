package rest

import (
	"net/http"
	"time"

	applicationidentity "github.com/fastygo/backend/internal/application/identity"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/framework/pkg/core"
)

type IdentityHandler struct {
	service   *applicationidentity.Service
	principal PrincipalResolver
}

func NewIdentityHandler(
	service *applicationidentity.Service,
	principal PrincipalResolver,
) (*IdentityHandler, error) {
	if service == nil {
		return nil, core.NewDomainError(core.ErrorCodeInternal, "identity service is required")
	}
	return &IdentityHandler{service: service, principal: principal}, nil
}

func (handler *IdentityHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /go-json/go/v2/auth/login", handler.login)
	mux.HandleFunc("GET /go-json/go/v2/users", handler.listUsers)
	mux.HandleFunc("POST /go-json/go/v2/users", handler.createUser)
	mux.HandleFunc("PUT /go-json/go/v2/users/{id}", handler.updateUser)
	mux.HandleFunc("DELETE /go-json/go/v2/users/{id}", handler.deleteUser)
	mux.HandleFunc("GET /go-json/go/v2/roles", handler.listRoles)
	mux.HandleFunc("POST /go-json/go/v2/roles", handler.createRole)
	mux.HandleFunc("PUT /go-json/go/v2/roles/{id}", handler.updateRole)
	mux.HandleFunc("DELETE /go-json/go/v2/roles/{id}", handler.deleteRole)
}

func (handler *IdentityHandler) login(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeTaxonomyJSON(response, request, &input); err != nil {
		writeError(response, request, err)
		return
	}
	token, err := handler.service.Authenticate(request.Context(), input.Email, input.Password, 24*time.Hour)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	writeJSON(response, http.StatusOK, map[string]any{
		"access_token": token, "token_type": "Bearer", "expires_in": 86400,
	})
}

func (handler *IdentityHandler) listUsers(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	users, err := handler.service.ListUsers(request.Context(), principal)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": projectUsers(users)})
}

func (handler *IdentityHandler) createUser(response http.ResponseWriter, request *http.Request) {
	handler.saveUser(response, request, 0)
}

func (handler *IdentityHandler) updateUser(response http.ResponseWriter, request *http.Request) {
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	handler.saveUser(response, request, version)
}

func (handler *IdentityHandler) saveUser(
	response http.ResponseWriter,
	request *http.Request,
	version uint64,
) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document userWriteDocument
	if err := decodeTaxonomyJSON(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	input := document.input()
	if version > 0 {
		input.ID = request.PathValue("id")
	}
	user, err := handler.service.SaveUser(request.Context(), principal, input, version)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(user.Version))
	status := http.StatusOK
	if version == 0 {
		status = http.StatusCreated
		response.Header().Set("Location", request.URL.Path+"/"+user.ID)
	}
	writeJSON(response, status, map[string]any{"data": projectUser(user)})
}

func (handler *IdentityHandler) deleteUser(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	if err := handler.service.DeleteUser(request.Context(), principal, request.PathValue("id"), version); err != nil {
		writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (handler *IdentityHandler) listRoles(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	roles, err := handler.service.ListRoles(request.Context(), principal)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": projectRoles(roles)})
}

func (handler *IdentityHandler) createRole(response http.ResponseWriter, request *http.Request) {
	handler.saveRole(response, request, 0)
}

func (handler *IdentityHandler) updateRole(response http.ResponseWriter, request *http.Request) {
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	handler.saveRole(response, request, version)
}

func (handler *IdentityHandler) saveRole(
	response http.ResponseWriter,
	request *http.Request,
	version uint64,
) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document persist.Role
	if err := decodeTaxonomyJSON(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	role := document.Domain()
	if version > 0 {
		role.ID = request.PathValue("id")
	}
	saved, err := handler.service.SaveRole(request.Context(), principal, role, version)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(saved.Version))
	status := http.StatusOK
	if version == 0 {
		status = http.StatusCreated
		response.Header().Set("Location", request.URL.Path+"/"+saved.ID)
	}
	writeJSON(response, status, map[string]any{"data": persist.RoleFromDomain(saved)})
}

func (handler *IdentityHandler) deleteRole(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	if err := handler.service.DeleteRole(request.Context(), principal, request.PathValue("id"), version); err != nil {
		writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (handler *IdentityHandler) resolvePrincipal(
	response http.ResponseWriter,
	request *http.Request,
) (authz.Principal, bool) {
	if handler.principal == nil {
		writeError(response, request, core.NewDomainError(core.ErrorCodeUnauthorized, "authentication is required"))
		return authz.Principal{}, false
	}
	principal, err := handler.principal.Resolve(request)
	if err != nil || principal.Anonymous {
		writeError(response, request, core.NewDomainError(core.ErrorCodeUnauthorized, "authentication is required"))
		return authz.Principal{}, false
	}
	return principal, true
}
