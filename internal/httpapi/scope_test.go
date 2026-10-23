package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/auth"
	"wa-gateway/internal/netguard"
)

// scopedRouter monta autenticação + sessionScopeMW como no router real. A
// chave "tenant.seg" só enxerga a sessão cliente-a; "admin.seg" tem "*".
// O handler devolve a sessão que um handler de verdade leria (path, query
// ou corpo decodificado com encoding/json) e se o contexto saiu estrito.
func scopedRouter(t *testing.T) http.Handler {
	t.Helper()
	hash, err := auth.HashKey("seg")
	if err != nil {
		t.Fatal(err)
	}
	a := auth.New("", func(_ context.Context, id string) (string, []string, error) {
		switch id {
		case "tenant":
			return hash, []string{"session:cliente-a"}, nil
		case "admin":
			return hash, []string{"*"}, nil
		}
		return "", nil, auth.ErrUnknownKey
	})
	echo := func(w http.ResponseWriter, r *http.Request) {
		sess := chi.URLParam(r, "session")
		if sess == "" {
			sess = r.URL.Query().Get("session")
		}
		var body struct {
			Session string `json:"session"`
		}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
		if body.Session != "" {
			sess = body.Session
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": sess, "strict": netguard.IsStrict(r.Context())})
	}
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(a.Middleware)
		r.Use(sessionScopeMW)
		r.Post("/api/sendText", echo)
		r.Post("/api/sendText#plain", echo)
		r.Get("/api/{session}/leads", echo)
		r.Get("/api/stats", echo)
		r.Get("/ws", echo)
	})
	return r
}

func scopedDo(t *testing.T, h http.Handler, key, method, url, body string) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, rd)
	req.Header.Set("X-Api-Key", key)
	if body != "" && !strings.HasSuffix(url, "#plain") {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestSessionScopeBlocksBypasses(t *testing.T) {
	h := scopedRouter(t)
	pad := strings.Repeat("A", 600<<10)
	cases := []struct {
		name, method, url, body string
	}{
		{"chave duplicada", "POST", "/api/sendText", `{"session":"cliente-a","session":"cliente-b","text":"x"}`},
		{"query x corpo", "POST", "/api/sendText?session=cliente-a", `{"session":"cliente-b","text":"x"}`},
		{"padding > 512KB", "POST", "/api/sendText", `{"file":"` + pad + `","session":"cliente-b"}`},
		{"maiúscula", "POST", "/api/sendText", `{"session":"cliente-a","Session":"cliente-b"}`},
		{"unicode fold (ſ)", "POST", "/api/sendText", `{"session":"cliente-a","ſession":"cliente-b"}`},
		{"query repetida", "GET", "/api/stats?session=cliente-a&session=cliente-b", ""},
		{"path de outro", "GET", "/api/cliente-b/leads", ""},
		{"sem content-type json", "POST", "/api/sendText#plain", `{"session":"cliente-b"}`},
		{"wildcard", "POST", "/api/sendText", `{"session":"*"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out := scopedDo(t, h, "tenant.seg", c.method, c.url, c.body)
			if code != http.StatusForbidden {
				t.Fatalf("status = %d (handler viu sessão %v), quero 403", code, out["session"])
			}
		})
	}
}

func TestSessionScopeAllowsOwnSession(t *testing.T) {
	h := scopedRouter(t)
	for _, c := range []struct{ method, url, body string }{
		{"POST", "/api/sendText", `{"session":"cliente-a","text":"oi"}`},
		{"POST", "/api/sendText?session=cliente-a", `{"session":"cliente-a"}`},
		{"GET", "/api/cliente-a/leads", ""},
		{"GET", "/api/stats", ""},
	} {
		code, out := scopedDo(t, h, "tenant.seg", c.method, c.url, c.body)
		if code != http.StatusOK {
			t.Fatalf("%s %s: status = %d, quero 200 (%v)", c.method, c.url, code, out)
		}
		if out["strict"] != true {
			t.Errorf("%s %s: chave de tenant deveria sair com contexto anti-SSRF estrito", c.method, c.url)
		}
	}
}

func TestSessionScopeRejectsBadJSONAndNonStringSession(t *testing.T) {
	h := scopedRouter(t)
	for _, body := range []string{`{"session":"cliente-a"`, `{"session":["cliente-b"]}`} {
		if code, _ := scopedDo(t, h, "tenant.seg", "POST", "/api/sendText", body); code != http.StatusBadRequest {
			t.Errorf("corpo %q: status = %d, quero 400", body, code)
		}
	}
}

func TestSessionScopeWSNeedsSession(t *testing.T) {
	h := scopedRouter(t)
	if code, _ := scopedDo(t, h, "tenant.seg", "GET", "/ws", ""); code != http.StatusForbidden {
		t.Fatalf("/ws sem sessão: status = %d, quero 403", code)
	}
	if code, _ := scopedDo(t, h, "tenant.seg", "GET", "/ws?session=cliente-b", ""); code != http.StatusForbidden {
		t.Fatalf("/ws de outro: status = %d, quero 403", code)
	}
}

func TestSessionScopeAdminUntouched(t *testing.T) {
	h := scopedRouter(t)
	code, out := scopedDo(t, h, "admin.seg", "POST", "/api/sendText", `{"session":"cliente-b"}`)
	if code != http.StatusOK || out["session"] != "cliente-b" {
		t.Fatalf("admin: status=%d out=%v", code, out)
	}
	if out["strict"] == true {
		t.Error("chave admin não deveria ficar no modo estrito")
	}
}

// mcpAs roda o mcpHandler autenticado com a chave dada.
func mcpAs(t *testing.T, key, body string) map[string]any {
	t.Helper()
	hash, _ := auth.HashKey("seg")
	a := auth.New("", func(_ context.Context, id string) (string, []string, error) {
		return hash, []string{"session:cliente-a"}, nil
	})
	h := a.Middleware(http.HandlerFunc(Deps{}.mcpHandler))
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("X-Api-Key", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta não-JSON: %s", rec.Body.String())
	}
	return out
}

func TestMCPScopeDenied(t *testing.T) {
	for _, args := range []string{
		`{"session":"cliente-b","chatId":"1","text":"x"}`,
		`{"session":"cliente-a","Session":"cliente-b","chatId":"1","text":"x"}`,
		`{"chatId":"1","text":"x"}`,
	} {
		out := mcpAs(t, "tenant.seg", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"send_text","arguments":`+args+`}}`)
		res, _ := out["result"].(map[string]any)
		txt := ""
		if c, _ := res["content"].([]any); len(c) > 0 {
			txt, _ = c[0].(map[string]any)["text"].(string)
		}
		if res["isError"] != true || strings.Contains(txt, "não está ativa") {
			t.Errorf("args %s: deveria recusar por escopo antes de chegar na engine, veio %v", args, out)
		}
	}
	out := mcpAs(t, "tenant.seg", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_session","arguments":{"session":"cliente-a"}}}`)
	if res, _ := out["result"].(map[string]any); res["isError"] != true {
		t.Errorf("chave escopada não pode criar sessão: %v", out)
	}
}
