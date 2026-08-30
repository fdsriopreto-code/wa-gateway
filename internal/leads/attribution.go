package leads

import (
	"net/url"
	"regexp"
	"strings"
)

// Source é a atribuição de origem de um lead, montada no 1º contato.
type Source struct {
	AdReferral   map[string]any    `json:"adReferral,omitempty"` // anúncio Click-to-WhatsApp (id, url, ctwa_clid…)
	UTM          map[string]string `json:"utm,omitempty"`        // utm_source, utm_campaign, …
	ClickIDs     map[string]string `json:"clickIds,omitempty"`   // gclid, fbclid, msclkid, ttclid…
	Params       map[string]string `json:"params,omitempty"`     // ref, src, lead_source…
	FirstMessage string            `json:"firstMessage,omitempty"`
}

// Has diz se achou algo de atribuição de verdade (fora o texto).
func (s Source) Has() bool {
	return len(s.AdReferral) > 0 || len(s.UTM) > 0 || len(s.ClickIDs) > 0 || len(s.Params) > 0
}

var (
	// key=value soltos no texto OU em querystrings (o & separa naturalmente).
	kvRe    = regexp.MustCompile(`(?i)\b([a-z][\w.\-]{1,40})=([^\s&?#,;"'<>)\]]{1,240})`)
	urlRe   = regexp.MustCompile(`https?://[^\s"'<>)\]]+`)
	clickID = map[string]bool{"gclid": true, "fbclid": true, "gbraid": true, "wbraid": true, "msclkid": true, "ttclid": true, "igshid": true, "dclid": true}
	namedP  = map[string]bool{"ref": true, "src": true, "source": true, "campaign": true, "lead_source": true, "adset": true, "ad": true, "placement": true, "keyword": true, "kw": true, "cid": true}
)

// ExtractSource lê o payload de uma mensagem RECEBIDA e monta a atribuição.
// - adReferral vem das engines (contextInfo.externalAdReply / referral da Meta)
// - UTM / click ids / params saem do texto (wa.me?text=…, link colado, etc.)
func ExtractSource(p map[string]any) Source {
	var s Source
	if ad, ok := p["adReferral"].(map[string]any); ok && len(ad) > 0 {
		s.AdReferral = ad
	}
	body, _ := p["body"].(string)
	s.FirstMessage = trunc(body, 400)

	pairs := map[string]string{}
	collect := func(text string) {
		for _, mm := range kvRe.FindAllStringSubmatch(text, -1) {
			k := strings.ToLower(mm[1])
			v := decode(mm[2])
			if v != "" && pairs[k] == "" {
				pairs[k] = v
			}
		}
	}
	collect(body)
	// querystrings de URLs coladas (mais confiável que o regex solto)
	for _, u := range urlRe.FindAllString(body, -1) {
		if pu, err := url.Parse(u); err == nil {
			for k, vs := range pu.Query() {
				lk := strings.ToLower(k)
				if len(vs) > 0 && pairs[lk] == "" {
					pairs[lk] = vs[0]
				}
			}
		}
	}

	utm := map[string]string{}
	click := map[string]string{}
	other := map[string]string{}
	for k, v := range pairs {
		switch {
		case strings.HasPrefix(k, "utm_"):
			utm[k] = v
		case clickID[k]:
			click[k] = v
		case namedP[k]:
			other[k] = v
		}
	}
	if len(utm) > 0 {
		s.UTM = utm
	}
	if len(click) > 0 {
		s.ClickIDs = click
	}
	if len(other) > 0 {
		s.Params = other
	}
	return s
}

func decode(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return strings.TrimSpace(d)
	}
	return strings.TrimSpace(s)
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
