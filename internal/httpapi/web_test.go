package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandler(t *testing.T) {
	h := spaHandler()

	cases := []struct {
		path       string
		wantCode   int
		wantCT     string
		wantInBody string
	}{
		{"/", 200, "text/html", "<title>wa-gateway"},
		{"/app.js", 200, "", "views.playground"},
		{"/styles.css", 200, "text/css", "--ac:"},
		{"/rota/inexistente", 200, "text/html", "wa-gateway"}, // fallback SPA
		{"/api/qualquer", 404, "application/json", "not_found"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
			if rec.Code != c.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, c.wantCode)
			}
			if c.wantCT != "" && !strings.Contains(rec.Header().Get("Content-Type"), c.wantCT) {
				t.Errorf("content-type = %q, want ~%q", rec.Header().Get("Content-Type"), c.wantCT)
			}
			if c.wantInBody != "" && !strings.Contains(rec.Body.String(), c.wantInBody) {
				t.Errorf("body sem %q", c.wantInBody)
			}
		})
	}
}
