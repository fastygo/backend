package schema

import (
	"strings"
	"testing"
)

func TestManifestGeneratesJSONSchemaGraphQLAndOpenAPI(t *testing.T) {
	manifest := productManifest()
	jsonSchema, err := manifest.JSONSchema("product")
	if err != nil {
		t.Fatalf("generate JSON Schema: %v", err)
	}
	properties := jsonSchema["properties"].(map[string]any)
	title := properties["title"].(map[string]any)
	if title["type"] != "object" || title["patternProperties"] == nil {
		t.Fatalf("localized field was not represented in JSON Schema")
	}

	sdl, err := manifest.GraphQLSDL()
	if err != nil {
		t.Fatalf("generate GraphQL SDL: %v", err)
	}
	for _, expected := range []string{"type Product", "products(", "createProduct", "categories: [ID!]"} {
		if !strings.Contains(sdl, expected) {
			t.Fatalf("GraphQL SDL does not contain %q:\n%s", expected, sdl)
		}
	}

	openAPI, err := manifest.OpenAPI()
	if err != nil {
		t.Fatalf("generate OpenAPI: %v", err)
	}
	if openAPI["openapi"] != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version")
	}
	paths := openAPI["paths"].(map[string]any)
	if paths["/go-json/go/v2/products"] == nil {
		t.Fatalf("resource path is missing from OpenAPI")
	}
	recordPath, ok := paths["/go-json/go/v2/products/{id}"].(map[string]any)
	if !ok || recordPath["put"] != nil || recordPath["patch"] == nil {
		t.Fatalf("record methods do not match the REST contract: %#v", recordPath)
	}
	for _, path := range []string{
		"/go-json/go/v2/products/form",
		"/go-json/go/v2/products/by-slug/{slug}",
		"/go-json/go/v2/products/{id}/revisions",
		"/go-json/go/v2/products/{id}/revisions/{revision}/restore",
		"/go-json/go/v2/products/{id}/form",
	} {
		if paths[path] == nil {
			t.Fatalf("canonical collection endpoint missing from OpenAPI: %s", path)
		}
	}
}
