package leads

import (
	"testing"
	"time"
)

func TestExtractSource_AdReferral(t *testing.T) {
	s := ExtractSource(map[string]any{
		"body": "Olá, tenho interesse",
		"adReferral": map[string]any{
			"sourceId": "120210000111", "sourceType": "ad",
			"sourceUrl": "https://fb.me/abc", "ctwaClid": "AR-xyz",
		},
	})
	if s.AdReferral["sourceId"] != "120210000111" || !s.Has() {
		t.Fatalf("adReferral não passou: %+v", s)
	}
	if s.FirstMessage != "Olá, tenho interesse" {
		t.Fatalf("firstMessage=%q", s.FirstMessage)
	}
}

func TestExtractSource_UTMInText(t *testing.T) {
	s := ExtractSource(map[string]any{
		"body": "vim pelo site https://loja.com/promo?utm_source=instagram&utm_campaign=julho-2026&fbclid=IwAR123 quero saber preço",
	})
	if s.UTM["utm_source"] != "instagram" || s.UTM["utm_campaign"] != "julho-2026" {
		t.Fatalf("utm errado: %+v", s.UTM)
	}
	if s.ClickIDs["fbclid"] != "IwAR123" {
		t.Fatalf("fbclid errado: %+v", s.ClickIDs)
	}
	if !s.Has() {
		t.Fatal("Has() devia ser true")
	}
}

func TestExtractSource_BareTokens(t *testing.T) {
	// wa.me/...?text= costuma chegar como texto puro
	s := ExtractSource(map[string]any{"body": "quero orçamento ref=landing-home utm_medium=cpc gclid=Cj0KxyZ"})
	if s.Params["ref"] != "landing-home" {
		t.Fatalf("ref: %+v", s.Params)
	}
	if s.UTM["utm_medium"] != "cpc" {
		t.Fatalf("utm_medium: %+v", s.UTM)
	}
	if s.ClickIDs["gclid"] != "Cj0KxyZ" {
		t.Fatalf("gclid: %+v", s.ClickIDs)
	}
}

func TestExtractSource_NothingIsFalse(t *testing.T) {
	s := ExtractSource(map[string]any{"body": "oi tudo bem? preciso de ajuda"})
	if s.Has() {
		t.Fatalf("não devia achar atribuição: %+v", s)
	}
	if s.FirstMessage == "" {
		t.Fatal("firstMessage devia guardar o texto")
	}
}

func TestExtractSource_URLDecode(t *testing.T) {
	s := ExtractSource(map[string]any{"body": "https://x.com/?utm_campaign=promo%20de%20ver%C3%A3o"})
	if s.UTM["utm_campaign"] != "promo de verão" {
		t.Fatalf("decode falhou: %q", s.UTM["utm_campaign"])
	}
}

func TestPreviewAndHelpers(t *testing.T) {
	if preview(map[string]any{"body": "  ola  "}) != "ola" {
		t.Fatal("preview texto")
	}
	if preview(map[string]any{"type": "image"}) != "[imagem]" {
		t.Fatal("preview mídia")
	}
	if !is1to1("5511999@s.whatsapp.net") || is1to1("123@g.us") {
		t.Fatal("is1to1")
	}
	if digits("+55 (17) 98117-9818@s.whatsapp.net") != "5517981179818" {
		t.Fatal("digits")
	}
	if tsOf(float64(1730000000)).Unix() != 1730000000 {
		t.Fatal("tsOf float")
	}
	if !tsOf("2026-08-30T12:00:00Z").Equal(time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("tsOf rfc3339")
	}
}
