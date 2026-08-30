package auth

import "testing"

func TestPrincipalScopes(t *testing.T) {
	cases := []struct {
		name       string
		scopes     []string
		session    string
		canSession bool
		scoped     bool
	}{
		{"master", []string{"*"}, "vendas", true, false},
		{"session wildcard", []string{"session:*"}, "vendas", true, false},
		{"escopo exato", []string{"session:vendas"}, "vendas", true, true},
		{"escopo exato nega outra", []string{"session:vendas"}, "suporte", false, true},
		{"multi escopo", []string{"session:vendas", "session:suporte"}, "suporte", true, true},
		{"sem escopo de sessão", []string{"read"}, "vendas", false, false},
		{"vazio", nil, "vendas", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Principal{Scopes: c.scopes}
			if got := p.CanSession(c.session); got != c.canSession {
				t.Errorf("CanSession(%q) = %v, want %v", c.session, got, c.canSession)
			}
			if got := p.SessionScoped(); got != c.scoped {
				t.Errorf("SessionScoped() = %v, want %v", got, c.scoped)
			}
		})
	}
}
