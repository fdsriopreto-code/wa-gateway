package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/session"
)

// header anti-loop: um request já encaminhado não é encaminhado de novo.
const fwdHeader = "X-WA-Forwarded"

// clusterProxyMW encaminha o request para o nó dono da sessão quando ela não
// está viva localmente. Só liga com NODE_ADVERTISE_URL setado — sem isso é um
// passa-direto de custo ~zero (um if).
func clusterProxyMW(mgr *session.Manager, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !mgr.ClusterEnabled() || r.Header.Get(fwdHeader) == "1" || r.URL.Path == "/ws" {
				next.ServeHTTP(w, r)
				return
			}
			sess := sessionFromRequest(r)
			if sess == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := mgr.Engine(sess); ok {
				next.ServeHTTP(w, r) // é nossa
				return
			}
			base, remote := mgr.OwnerBaseURL(r.Context(), sess)
			if !remote {
				next.ServeHTTP(w, r) // ninguém é dono (ou o dono não se anunciou): handler local responde
				return
			}
			proxyTo(w, r, base, log)
		})
	}
}

// sessionFromRequest tira o nome da sessão de: path param {session}, query
// ?session=, ou corpo JSON {"session": "..."}. No caso do corpo, faz buffer e
// restaura pra não consumir o Body do handler / proxy.
func sessionFromRequest(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if s := rc.URLParam("session"); s != "" {
			return s
		}
	}
	if s := r.URL.Query().Get("session"); s != "" {
		return s
	}
	if r.Body == nil || r.Method == http.MethodGet ||
		!strings.Contains(r.Header.Get("Content-Type"), "json") {
		return ""
	}
	// só espia os primeiros 512KB (o campo "session" fica sempre no topo do
	// JSON) e devolve o corpo intacto via MultiReader — não bufferiza os
	// 32MB de um envio de mídia.
	const peek = 512 << 10
	head := make([]byte, peek)
	n, _ := io.ReadFull(r.Body, head)
	head = head[:n]
	orig := r.Body
	r.Body = readCloser{io.MultiReader(bytes.NewReader(head), orig), orig}
	return topLevelString(head, "session")
}

type readCloser struct {
	io.Reader
	io.Closer
}

// topLevelString varre um JSON (possivelmente truncado) e devolve o valor
// string da chave `key` no objeto raiz. "" se não achar / truncou antes.
func topLevelString(b []byte, key string) string {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return ""
	}
	for {
		kt, err := dec.Token() // chave (ou '}')
		if err != nil || kt == json.Delim('}') {
			return ""
		}
		name, ok := kt.(string)
		if !ok {
			return ""
		}
		vt, err := dec.Token() // valor
		if err != nil {
			return ""
		}
		if d, ok := vt.(json.Delim); ok && (d == '{' || d == '[') {
			if skipNested(dec) != nil {
				return ""
			}
			continue
		}
		if name == key {
			s, _ := vt.(string)
			return s
		}
	}
}

func skipNested(dec *json.Decoder) error {
	for depth := 1; depth > 0; {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
	return nil
}

func proxyTo(w http.ResponseWriter, r *http.Request, base string, log *slog.Logger) {
	target, err := url.Parse(base)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "cluster_proxy", "endereço do nó dono inválido: "+base)
		return
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	orig := rp.Director
	rp.Director = func(req *http.Request) {
		orig(req)
		req.Host = target.Host
		req.Header.Set(fwdHeader, "1")
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, e error) {
		if log != nil {
			log.Warn("cluster proxy falhou", "target", base, "err", e)
		}
		writeErr(w, http.StatusBadGateway, "cluster_proxy", "falha ao falar com o nó dono da sessão: "+e.Error())
	}
	if log != nil {
		log.Debug("encaminhando p/ nó dono", "target", base, "path", r.URL.Path)
	}
	rp.ServeHTTP(w, r)
}
