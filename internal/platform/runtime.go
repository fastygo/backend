package platform

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/framework/pkg/app"
	"github.com/fastygo/panel"
)

// Capability is the stable permission identifier shared by services and Panel.
type Capability = authz.Capability

// Principal is the authorization projection shared by services and Panel.
type Principal = authz.Principal

// ControlPlane contains metadata consumed by any compatible admin frontend.
type ControlPlane struct {
	Panel *panel.Panel[Principal, Capability]
}

type RouteRegistrar interface {
	Routes(*http.ServeMux)
}

type RouteGroup []RouteRegistrar

func (group RouteGroup) Routes(mux *http.ServeMux) {
	for _, routes := range group {
		if routes != nil {
			routes.Routes(mux)
		}
	}
}

type HealthResource interface {
	Ping(context.Context) error
}

type CloseResource interface {
	Close() error
}

// NewControlPlane creates a UI-neutral Panel registry.
func NewControlPlane(id, title, basePath string) (*ControlPlane, error) {
	admin, err := panel.NewPanel[Principal, Capability](panel.PanelOptions[Capability]{
		ID:       panel.PanelID(strings.TrimSpace(id)),
		Title:    strings.TrimSpace(title),
		BasePath: strings.TrimSpace(basePath),
	})
	if err != nil {
		return nil, err
	}
	return &ControlPlane{Panel: admin}, nil
}

// Feature adapts a headless HTTP handler to the Framework application runtime.
type Feature struct {
	id      string
	handler http.Handler
	nav     []app.NavItem
}

// NewFeature creates a compile-time Framework feature.
func NewFeature(id string, handler http.Handler, nav ...app.NavItem) (*Feature, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("feature id is required")
	}
	if handler == nil {
		return nil, errors.New("feature handler is required")
	}
	return &Feature{id: id, handler: handler, nav: append([]app.NavItem(nil), nav...)}, nil
}

// ID returns the stable feature identifier.
func (f *Feature) ID() string {
	return f.id
}

// Routes mounts the feature under the shared Framework mux.
func (f *Feature) Routes(mux *http.ServeMux) {
	mux.Handle("/", f.handler)
}

// NavItems returns a defensive copy of optional control-plane navigation.
func (f *Feature) NavItems() []app.NavItem {
	return append([]app.NavItem(nil), f.nav...)
}

// RuntimeFeature binds API routes and durable resources to Framework lifecycle contracts.
type RuntimeFeature struct {
	id        string
	routes    RouteRegistrar
	health    HealthResource
	closeable CloseResource
	nav       []app.NavItem
}

func NewRuntimeFeature(
	id string,
	routes RouteRegistrar,
	health HealthResource,
	closeable CloseResource,
	nav ...app.NavItem,
) (*RuntimeFeature, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("feature id is required")
	}
	if routes == nil {
		return nil, errors.New("feature routes are required")
	}
	return &RuntimeFeature{
		id: id, routes: routes, health: health, closeable: closeable,
		nav: append([]app.NavItem(nil), nav...),
	}, nil
}

func (feature *RuntimeFeature) ID() string {
	return feature.id
}

func (feature *RuntimeFeature) Routes(mux *http.ServeMux) {
	feature.routes.Routes(mux)
}

func (feature *RuntimeFeature) NavItems() []app.NavItem {
	return append([]app.NavItem(nil), feature.nav...)
}

func (feature *RuntimeFeature) HealthCheck(ctx context.Context) error {
	if feature.health == nil {
		return nil
	}
	return feature.health.Ping(ctx)
}

func (feature *RuntimeFeature) Close(context.Context) error {
	if feature.closeable == nil {
		return nil
	}
	return feature.closeable.Close()
}
