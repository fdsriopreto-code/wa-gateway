package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBlockedIP(t *testing.T) {
	Configure(false, nil)
	defer Configure(false, nil)
	cases := []struct {
		ip             string
		normal, strict bool // bloqueado?
	}{
		{"169.254.169.254", true, true}, // metadados da cloud
		{"fe80::1", true, true},
		{"0.0.0.0", true, true},
		{"224.0.0.1", true, true},
		{"127.0.0.1", false, true},
		{"10.1.2.3", false, true},
		{"192.168.0.10", false, true},
		{"172.16.5.5", false, true},
		{"100.64.1.1", false, true},
		{"::1", false, true},
		{"fd00::1", false, true},
		{"::ffff:127.0.0.1", false, true},
		{"8.8.8.8", false, false},
		{"2606:4700::1111", false, false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if got := blockedIP(ip, false); got != c.normal {
			t.Errorf("%s normal: bloqueado=%v, quero %v", c.ip, got, c.normal)
		}
		if got := blockedIP(ip, true); got != c.strict {
			t.Errorf("%s estrito: bloqueado=%v, quero %v", c.ip, got, c.strict)
		}
	}
}

func TestAllowList(t *testing.T) {
	Configure(true, []string{"10.0.5.0/24", "n8n", "192.168.1.7"})
	defer Configure(false, nil)
	if blockedIP(net.ParseIP("10.0.5.9"), true) {
		t.Error("CIDR liberado deveria passar")
	}
	if blockedIP(net.ParseIP("192.168.1.7"), true) {
		t.Error("IP liberado deveria passar")
	}
	if !blockedIP(net.ParseIP("10.0.6.1"), true) {
		t.Error("fora do CIDR liberado deveria bloquear")
	}
	if !hostAllowed("N8N.", "5678") {
		t.Error("hostname liberado deveria passar (sem diferenciar maiúscula/ponto final)")
	}
}

func TestDialStrictVsNormal(t *testing.T) {
	Configure(false, nil)
	defer Configure(false, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := Client(5 * time.Second)

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if resp, err := c.Do(req); err != nil {
		t.Fatalf("modo normal deveria alcançar loopback: %v", err)
	} else {
		resp.Body.Close()
	}

	req, _ = http.NewRequestWithContext(WithStrict(context.Background()), http.MethodGet, srv.URL, nil)
	if _, err := c.Do(req); err == nil || !errors.Is(err, ErrBlocked) {
		t.Fatalf("modo estrito deveria bloquear loopback, err=%v", err)
	}

	Configure(true, []string{"127.0.0.1"})
	req, _ = http.NewRequestWithContext(WithStrict(context.Background()), http.MethodGet, srv.URL, nil)
	if resp, err := c.Do(req); err != nil {
		t.Fatalf("loopback liberado deveria passar: %v", err)
	} else {
		resp.Body.Close()
	}
}

func TestRedirectToInternalBlocked(t *testing.T) {
	Configure(false, nil)
	defer Configure(false, nil)
	// redirect pra metadados: a checagem no dial pega o segundo salto.
	srv := httptest.NewServer(http.RedirectHandler("http://169.254.169.254/latest/meta-data/", http.StatusFound))
	defer srv.Close()
	_, err := Client(3 * time.Second).Get(srv.URL)
	if err == nil || !errors.Is(err, ErrBlocked) {
		t.Fatalf("redirect pra link-local deveria ser bloqueado, err=%v", err)
	}
}

func TestCheckURL(t *testing.T) {
	Configure(false, nil)
	defer Configure(false, nil)
	ctx := context.Background()
	for _, u := range []string{"ftp://x", "http://", "javascript:alert(1)", "http://169.254.169.254/x"} {
		if CheckURL(ctx, u, false) == nil {
			t.Errorf("%q deveria ser recusada", u)
		}
	}
	if err := CheckURL(ctx, "http://127.0.0.1:5678/hook", false); err != nil {
		t.Errorf("loopback fora do modo estrito deveria passar: %v", err)
	}
	if CheckURL(ctx, "http://127.0.0.1:5678/hook", true) == nil {
		t.Error("loopback no modo estrito deveria ser recusada")
	}
	if CheckURL(WithStrict(ctx), "http://10.0.0.1/hook", false) == nil {
		t.Error("contexto estrito deveria valer no CheckURL")
	}
	if err := CheckURL(ctx, "https://8.8.8.8/hook", true); err != nil {
		t.Errorf("IP público deveria passar: %v", err)
	}
}

func TestCheckProxyURL(t *testing.T) {
	Configure(false, nil)
	defer Configure(false, nil)
	ctx := context.Background()
	if err := CheckProxyURL(ctx, "socks5://u:p@8.8.8.8:1080", true); err != nil {
		t.Errorf("proxy público deveria passar: %v", err)
	}
	if CheckProxyURL(ctx, "socks5://127.0.0.1:6379", true) == nil {
		t.Error("proxy apontando pro Redis local deveria ser recusado no modo estrito")
	}
	if CheckProxyURL(ctx, "gopher://8.8.8.8", true) == nil {
		t.Error("esquema estranho deveria ser recusado")
	}
	if CheckURL(ctx, "socks5://8.8.8.8:1080", true) == nil {
		t.Error("webhook não aceita socks5")
	}
}
