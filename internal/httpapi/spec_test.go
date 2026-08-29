package httpapi

import (
	"strings"
	"testing"
)

func TestOpenAPIDoc(t *testing.T) {
	doc := Deps{Version: "9.9.9"}.openapiDoc("https", "example.com")

	if doc["openapi"] != "3.0.3" {
		t.Fatalf("openapi version = %v", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("sem paths")
	}
	// todo endpoint da spec precisa aparecer com o metodo certo
	for _, e := range specEndpoints {
		item, ok := paths[e.path].(map[string]any)
		if !ok {
			t.Errorf("path ausente: %s", e.path)
			continue
		}
		if _, ok := item[strings.ToLower(e.method)]; !ok {
			t.Errorf("%s %s ausente na spec", e.method, e.path)
		}
	}
	// path param vira parameter
	sess := paths["/api/sessions/{session}"].(map[string]any)["get"].(map[string]any)
	ps, _ := sess["parameters"].([]any)
	if len(ps) == 0 {
		t.Error("/api/sessions/{session} deveria ter o parametro de path")
	}
	// seguranca por header X-Api-Key
	comp := doc["components"].(map[string]any)["securitySchemes"].(map[string]any)["ApiKey"].(map[string]any)
	if comp["name"] != "X-Api-Key" {
		t.Errorf("securityScheme errado: %+v", comp)
	}
}
