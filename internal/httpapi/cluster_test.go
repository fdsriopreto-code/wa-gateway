package httpapi

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionFromRequest(t *testing.T) {
	t.Run("query", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/api/contacts?session=vendas", nil)
		if got := sessionFromRequest(r); got != "vendas" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("body json + restaura o corpo", func(t *testing.T) {
		body := `{"session":"suporte","chatId":"55@s.whatsapp.net","text":"oi"}`
		r := httptest.NewRequest("POST", "/api/sendText", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		if got := sessionFromRequest(r); got != "suporte" {
			t.Fatalf("got %q", got)
		}
		// o handler seguinte precisa conseguir ler o corpo inteiro
		rest, _ := io.ReadAll(r.Body)
		if string(rest) != body {
			t.Fatalf("corpo não restaurado: %q", rest)
		}
	})

	t.Run("sem session", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/api/stats", nil)
		if got := sessionFromRequest(r); got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("body não-json é ignorado", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/x", strings.NewReader("session=abc"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if got := sessionFromRequest(r); got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("session depois de campos aninhados", func(t *testing.T) {
		body := `{"opts":{"a":1,"b":[1,2,{"c":3}]},"tags":["x"],"session":"vendas","text":"oi"}`
		r := httptest.NewRequest("POST", "/api/sendText", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if got := sessionFromRequest(r); got != "vendas" {
			t.Fatalf("got %q", got)
		}
		rest, _ := io.ReadAll(r.Body)
		if string(rest) != body {
			t.Fatalf("corpo não restaurado: %q", rest)
		}
	})
}

func TestTopLevelString(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"session":"a"}`, "a"},
		{`{"x":1,"session":"b"}`, "b"},
		{`{"o":{"session":"nested"},"session":"root"}`, "root"},
		{`{"arr":[{"session":"x"}],"session":"y"}`, "y"},
		{`{"session":42}`, ""},       // não-string
		{`{"a":1,"b":`, ""},          // truncado antes do session
		{`{"a":{"deep":{"x":1}`, ""}, // truncado dentro de aninhado
		{`not json`, ""},
	}
	for _, c := range cases {
		if got := topLevelString([]byte(c.in), "session"); got != c.want {
			t.Errorf("topLevelString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
