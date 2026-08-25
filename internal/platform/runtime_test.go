package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fastygo/framework/pkg/app"
)

func TestControlPlaneAndFeatureContracts(t *testing.T) {
	controlPlane, err := NewControlPlane("content", "Content", "/go-admin")
	if err != nil {
		t.Fatalf("create control plane: %v", err)
	}
	if controlPlane.Panel.ID() != "content" || controlPlane.Panel.BasePath() != "/go-admin" {
		t.Fatalf("unexpected panel identity")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	feature, err := NewFeature("content-api", handler, app.NavItem{Label: "Content", Path: "/go-admin"})
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}

	mux := http.NewServeMux()
	feature.Routes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/go-json", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", response.Code)
	}

	items := feature.NavItems()
	items[0].Label = "mutated"
	if feature.NavItems()[0].Label != "Content" {
		t.Fatalf("navigation must be returned as a defensive copy")
	}
}

func TestRuntimeFeatureDelegatesFrameworkLifecycle(t *testing.T) {
	resource := &testResource{}
	feature, err := NewRuntimeFeature("content", testRoutes{}, resource, resource)
	if err != nil {
		t.Fatalf("create runtime feature: %v", err)
	}
	mux := http.NewServeMux()
	feature.Routes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/content", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("routes were not registered")
	}
	if err := feature.HealthCheck(context.Background()); err != nil || resource.pings != 1 {
		t.Fatalf("health resource was not checked")
	}
	if err := feature.Close(context.Background()); err != nil || !resource.closed {
		t.Fatalf("resource was not closed")
	}
}

type testRoutes struct{}

func (testRoutes) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /content", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
}

type testResource struct {
	pings  int
	closed bool
}

func (resource *testResource) Ping(context.Context) error {
	resource.pings++
	return nil
}

func (resource *testResource) Close() error {
	resource.closed = true
	return nil
}
