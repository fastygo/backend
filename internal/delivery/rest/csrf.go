package rest

import (
	"net/http"

	"github.com/fastygo/framework/pkg/core"
)

// forbidCSRF rejects a cookie-authenticated mutation that fails the token check.
// Bearer requests and resolvers without a cookie session are unchanged.
func forbidCSRF(response http.ResponseWriter, request *http.Request, principal any) bool {
	guard, ok := principal.(interface {
		ValidateCookieCSRF(*http.Request) error
	})
	if !ok || guard == nil {
		return false
	}
	if err := guard.ValidateCookieCSRF(request); err != nil {
		writeError(response, request, core.NewDomainError(core.ErrorCodeForbidden, "csrf token is invalid"))
		return true
	}
	return false
}
