package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mcpCall(t *testing.T, d Deps, body string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	d.mcpHandler(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta não-JSON: %s", rec.Body.String())
	}
	return out
}

func TestMCPInitialize(t *testing.T) {
	out := mcpCall(t, Deps{Version: "1.2.3"}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	res, _ := out["result"].(map[string]any)
	if res == nil || res["protocolVersion"] != mcpProtocol {
		t.Fatalf("initialize inválido: %v", out)
	}
	si, _ := res["serverInfo"].(map[string]any)
	if si["name"] != "wa-gateway" || si["version"] != "1.2.3" {
		t.Errorf("serverInfo errado: %v", si)
	}
}

func TestMCPToolsList(t *testing.T) {
	out := mcpCall(t, Deps{}, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	res, _ := out["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	if len(tools) < 8 {
		t.Fatalf("esperava >=8 ferramentas, veio %d", len(tools))
	}
	for _, tv := range tools {
		m := tv.(map[string]any)
		if m["name"] == "" || m["description"] == "" || m["inputSchema"] == nil {
			t.Errorf("ferramenta incompleta: %v", m)
		}
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	out := mcpCall(t, Deps{}, `{"jsonrpc":"2.0","id":3,"method":"foo/bar"}`)
	if out["error"] == nil {
		t.Errorf("esperava erro JSON-RPC para método desconhecido: %v", out)
	}
}

func TestMCPUnknownTool(t *testing.T) {
	out := mcpCall(t, Deps{}, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if out["error"] == nil {
		t.Errorf("esperava erro para ferramenta inexistente: %v", out)
	}
}
