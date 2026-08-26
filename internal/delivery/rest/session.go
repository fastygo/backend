package rest

import (
	"net/http"
	"strings"
	"time"

	applicationidentity "github.com/fastygo/backend/internal/application/identity"
	tokenidentity "github.com/fastygo/backend/internal/identity"
	"github.com/fastygo/framework/pkg/core"
)

type SessionHandler struct {
	service      *applicationidentity.Service
	tokens       *tokenidentity.TokenManager
	cookieSecure bool
}

func NewSessionHandler(
	service *applicationidentity.Service,
	tokens *tokenidentity.TokenManager,
	cookieSecure bool,
) (*SessionHandler, error) {
	if service == nil || tokens == nil {
		return nil, core.NewDomainError(core.ErrorCodeInternal, "session authentication is required")
	}
	return &SessionHandler{service: service, tokens: tokens, cookieSecure: cookieSecure}, nil
}

func (handler *SessionHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /go-json/auth/login", handler.login)
	mux.HandleFunc("GET /go-json/auth/me", handler.me)
	mux.HandleFunc("POST /go-json/auth/logout", handler.logout)
}

func (handler *SessionHandler) login(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeTaxonomyJSON(response, request, &input); err != nil {
		writeError(response, request, err)
		return
	}
	session, err := handler.service.SignIn(request.Context(), input.Email, input.Password, 24*time.Hour)
	if err != nil {
		writeError(response, request, err)
		return
	}
	handler.writeSessionCookie(response, session.Token, 24*time.Hour)
	handler.writeSession(response, session.User.ID, session.User.Email, session.Token)
}

func (handler *SessionHandler) me(response http.ResponseWriter, request *http.Request) {
	principal, err := handler.tokens.Resolve(request)
	if err != nil || principal.Anonymous {
		writeError(response, request, core.NewDomainError(core.ErrorCodeUnauthorized, "authentication is required"))
		return
	}
	user, err := handler.service.CurrentUser(request.Context(), principal)
	if err != nil {
		writeError(response, request, err)
		return
	}
	token, _ := request.Cookie(tokenidentity.SessionCookieName)
	csrfSource := ""
	if token != nil {
		csrfSource = token.Value
	}
	if csrfSource == "" {
		_, token, found := strings.Cut(request.Header.Get("Authorization"), " ")
		if found {
			csrfSource = strings.TrimSpace(token)
		}
	}
	handler.writeSession(response, user.ID, user.Email, csrfSource)
}

func (handler *SessionHandler) logout(response http.ResponseWriter, request *http.Request) {
	if err := handler.tokens.ValidateCookieCSRF(request); err != nil {
		writeError(response, request, core.NewDomainError(core.ErrorCodeForbidden, "csrf token is invalid"))
		return
	}
	handler.writeSessionCookie(response, "", -1)
	response.WriteHeader(http.StatusNoContent)
}

func (handler *SessionHandler) writeSession(response http.ResponseWriter, id, email, token string) {
	response.Header().Set("Cache-Control", "no-store")
	writeJSON(response, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id": id, "email": email, "csrfToken": handler.tokens.CSRF(token),
		},
	})
}

func (handler *SessionHandler) writeSessionCookie(response http.ResponseWriter, token string, ttl time.Duration) {
	cookie := &http.Cookie{
		Name: tokenidentity.SessionCookieName, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: handler.cookieSecure,
	}
	if ttl < 0 {
		cookie.MaxAge = -1
	} else {
		cookie.MaxAge = int(ttl.Seconds())
	}
	http.SetCookie(response, cookie)
}

