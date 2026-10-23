// Package netguard protege as chamadas HTTP de saída cujo destino vem de
// fora (webhook, callbackUrl, mídia por URL, link preview) contra SSRF: um
// cliente apontando a URL pra rede interna (Redis, Postgres, metadados da
// cloud em 169.254.169.254...).
//
// A checagem é feita no DIAL, depois do DNS — então redirect e DNS rebinding
// não escapam. Duas camadas:
//
//   - sempre bloqueado: link-local (inclui metadados da cloud), multicast,
//     endereço não especificado. Ninguém tem webhook legítimo nesses.
//   - modo estrito: bloqueia também loopback, redes privadas (RFC1918, ULA) e
//     CGNAT. Liga globalmente com SSRF_BLOCK_PRIVATE=true ou por request com
//     WithStrict (o httpapi liga pra chaves de API de cliente/tenant).
//
// SSRF_ALLOW_HOSTS libera hosts/CIDRs internos conhecidos (ex.: "n8n,10.0.5.0/24").
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var ErrBlocked = errors.New("destino bloqueado (rede interna)")

type policy struct {
	blockPrivate bool
	hosts        map[string]bool // hostname ou host:port, minúsculo
	nets         []*net.IPNet
}

var current atomic.Pointer[policy]

func init() { Configure(false, nil) }

// Configure define a política global. allow aceita hostnames, host:port,
// IPs e CIDRs. O host do HTTP(S)_PROXY do ambiente entra liberado
// automaticamente, só na porta dele (senão o modo estrito barraria o
// próprio proxy). Destino que sai pelo proxy é checado no CheckURL, não no dial.
func Configure(blockPrivate bool, allow []string) {
	p := &policy{blockPrivate: blockPrivate, hosts: map[string]bool{}}
	for _, env := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		// só host:porta do proxy — liberar o host inteiro abriria todas as
		// portas dele (ex.: proxy em 127.0.0.1 liberaria o loopback todo).
		if u, err := url.Parse(os.Getenv(env)); err == nil && u.Hostname() != "" {
			port := u.Port()
			if port == "" {
				port = map[string]string{"https": "443", "socks5": "1080"}[u.Scheme]
				if port == "" {
					port = "80"
				}
			}
			p.hosts[strings.ToLower(u.Hostname())+":"+port] = true
		}
	}
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(a))
		switch {
		case a == "":
		case strings.Contains(a, "/"):
			if _, n, err := net.ParseCIDR(a); err == nil {
				p.nets = append(p.nets, n)
			}
		default:
			if ip := net.ParseIP(a); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				p.nets = append(p.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
				continue
			}
			p.hosts[a] = true
		}
	}
	current.Store(p)
}

type strictKey struct{}

// WithStrict marca o contexto: conexões abertas com ele usam o modo estrito
// mesmo com SSRF_BLOCK_PRIVATE desligado.
func WithStrict(ctx context.Context) context.Context {
	return context.WithValue(ctx, strictKey{}, true)
}

// IsStrict diz se conexões abertas com este contexto usam o modo estrito.
func IsStrict(ctx context.Context) bool { return isStrict(ctx) }

func isStrict(ctx context.Context) bool {
	v, _ := ctx.Value(strictKey{}).(bool)
	return v || current.Load().blockPrivate
}

var cgnat = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// blockedIP diz se o IP é proibido nesta política.
func blockedIP(ip net.IP, strict bool) bool {
	p := current.Load()
	for _, n := range p.nets {
		if n.Contains(ip) {
			return false
		}
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || (len(ip) == 4 && ip[0] == 0) {
		return true
	}
	if strict && (ip.IsLoopback() || ip.IsPrivate() || cgnat.Contains(ip)) {
		return true
	}
	return false
}

func hostAllowed(host, port string) bool {
	p := current.Load()
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return p.hosts[h] || p.hosts[h+":"+port]
}

var dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

// DialContext resolve o host, recusa IPs proibidos e conecta direto no IP
// checado (sem segunda resolução — fecha a janela de DNS rebinding).
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if hostAllowed(host, port) {
		return dialer.DialContext(ctx, network, addr)
	}
	strict := isStrict(ctx)
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error = fmt.Errorf("%w: %s", ErrBlocked, host)
	for _, ia := range ips {
		if blockedIP(ia.IP, strict) {
			continue
		}
		c, err := dialer.DialContext(ctx, network, net.JoinHostPort(ia.IP.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// Transport devolve um RoundTripper com o dial protegido. Requests em
// contexto estrito usam um pool de conexões separado: senão um request
// estrito reaproveitaria (keep-alive) uma conexão interna aberta por um
// request normal, sem passar pelo dial.
func Transport() http.RoundTripper {
	mk := func() *http.Transport {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.DialContext = DialContext
		return t
	}
	return &splitRT{normal: mk(), strict: mk()}
}

type splitRT struct{ normal, strict *http.Transport }

func (s *splitRT) RoundTrip(r *http.Request) (*http.Response, error) {
	if isStrict(r.Context()) {
		return s.strict.RoundTrip(r)
	}
	return s.normal.RoundTrip(r)
}

// Client é um http.Client com o dial protegido e o timeout dado.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: Transport()}
}

// CheckURL valida uma URL na hora em que ela é cadastrada (webhook,
// callbackUrl): só http(s) e o host não pode resolver pra rede proibida.
// strict força o modo estrito (usado pra chaves de tenant).
func CheckURL(ctx context.Context, raw string, strict bool) error {
	return checkURL(ctx, raw, strict, "http", "https")
}

// CheckProxyURL valida o proxy de uma sessão (socks5/http/https) com a mesma
// regra: um tenant não pode usar o "proxy" pra abrir conexão na rede interna.
func CheckProxyURL(ctx context.Context, raw string, strict bool) error {
	return checkURL(ctx, raw, strict, "socks5", "socks5h", "http", "https")
}

var defaultPorts = map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}

func checkURL(ctx context.Context, raw string, strict bool, schemes ...string) error {
	u, err := url.Parse(raw)
	okScheme := false
	if err == nil {
		for _, s := range schemes {
			okScheme = okScheme || u.Scheme == s
		}
	}
	if !okScheme || u.Hostname() == "" {
		return fmt.Errorf("url inválida (esquemas aceitos: %s): %q", strings.Join(schemes, ", "), raw)
	}
	port := u.Port()
	if port == "" {
		port = defaultPorts[u.Scheme]
	}
	if hostAllowed(u.Hostname(), port) {
		return nil
	}
	strict = strict || isStrict(ctx)
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(cctx, u.Hostname())
	if err != nil {
		if !strict {
			return nil // fora do modo estrito o dial confere na hora da entrega
		}
		return fmt.Errorf("não resolvi o host de %q: %w", u.Redacted(), err)
	}
	for _, ia := range ips {
		if blockedIP(ia.IP, strict) {
			return fmt.Errorf("%w: %s → %s", ErrBlocked, u.Hostname(), ia.IP)
		}
	}
	return nil
}
