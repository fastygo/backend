package bootstrap

import (
	"net/http"

	"github.com/fastygo/backend/internal/delivery/rest"
	"github.com/fastygo/backend/internal/tenant"
)

// hostMux dispatches by exact Host. In single mode the implicit default tenant
// handles every host. X-Forwarded-Host is never consulted.
type hostMux struct {
	multi    bool
	byHost   map[string]*site
	fallback *site
}

func (mux *hostMux) Routes(std *http.ServeMux) {
	std.Handle("/", mux)
}

func (mux *hostMux) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	site, ok := mux.lookup(request.Host)
	if !ok {
		rest.WriteUnknownHost(response, request)
		return
	}
	site.mux.ServeHTTP(response, request)
}

func (mux *hostMux) lookup(host string) (*site, bool) {
	key := tenant.NormalizeHost(host)
	if site, ok := mux.byHost[key]; ok {
		return site, true
	}
	if !mux.multi && mux.fallback != nil {
		return mux.fallback, true
	}
	return nil, false
}
