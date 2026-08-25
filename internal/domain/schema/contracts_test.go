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
	if paths["/go-json/data/v1/resources/product"] == nil {
		t.Fatalf("resource path is missing from OpenAPI")
	}
}
