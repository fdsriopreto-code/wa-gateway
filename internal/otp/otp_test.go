package otp

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"wa-gateway/internal/cache"
)

func newSvc(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rc, err := cache.New(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	return New(rc, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), mr
}

func TestDigits(t *testing.T) {
	for in, want := range map[string]string{
		"+55 (17) 98117-9818":          "5517981179818",
		"5517981179818@s.whatsapp.net": "5517981179818",
		"abc":                          "",
	} {
		if got := Digits(in); got != want {
			t.Errorf("Digits(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSendVerifyHappyPath(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()

	ch, err := s.Send(ctx, "sess", "+55 17 98117-9818", Options{Brand: "ACME"})
	if err != nil {
		t.Fatal(err)
	}
	if ch.To != "5517981179818" || ch.ID == "" || ch.code == "" || len(ch.code) != 6 {
		t.Fatalf("challenge ruim: %+v", ch)
	}
	if wantSub := ch.code; !contains(ch.Message, wantSub) || !contains(ch.Message, "ACME") {
		t.Fatalf("mensagem não renderizou: %q", ch.Message)
	}

	// código errado
	r, _ := s.Verify(ctx, "sess", "5517981179818", "", "000000")
	if r.Valid || r.Reason != "mismatch" || r.AttemptsLeft != 4 {
		t.Fatalf("mismatch esperado, veio %+v", r)
	}
	// código certo
	r, _ = s.Verify(ctx, "sess", "5517981179818", "", ch.code)
	if !r.Valid {
		t.Fatalf("devia validar, veio %+v", r)
	}
	// consumido: segunda vez não acha
	r, _ = s.Verify(ctx, "sess", "5517981179818", "", ch.code)
	if r.Valid || r.Reason != "not_found" {
		t.Fatalf("devia ter sido consumido, veio %+v", r)
	}
}

func TestVerifyByID(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	ch, _ := s.Send(ctx, "sess", "5511999999999", Options{})
	r, _ := s.Verify(ctx, "", "", ch.ID, ch.code)
	if !r.Valid {
		t.Fatalf("verify por id devia validar: %+v", r)
	}
}

func TestLockAfterMaxAttempts(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	ch, _ := s.Send(ctx, "sess", "5511888888888", Options{MaxAttempts: 3})
	for i := 0; i < 3; i++ {
		s.Verify(ctx, "sess", "5511888888888", "", "999999")
	}
	// agora nem o código certo passa
	r, _ := s.Verify(ctx, "sess", "5511888888888", "", ch.code)
	if r.Valid || r.Reason != "locked" {
		t.Fatalf("devia estar travado, veio %+v", r)
	}
}

func TestResendCooldown(t *testing.T) {
	s, mr := newSvc(t)
	ctx := context.Background()
	if _, err := s.Send(ctx, "sess", "5511777777777", Options{ResendAfter: 60 * time.Second}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Send(ctx, "sess", "5511777777777", Options{ResendAfter: 60 * time.Second})
	re, ok := err.(*RateError)
	if !ok || re.Reason != "cooldown" {
		t.Fatalf("esperava cooldown, veio %v", err)
	}
	// passa o tempo -> reenvia
	mr.FastForward(61 * time.Second)
	if _, err := s.Send(ctx, "sess", "5511777777777", Options{ResendAfter: 60 * time.Second}); err != nil {
		t.Fatalf("depois do cooldown devia reenviar: %v", err)
	}
}

func TestExpired(t *testing.T) {
	s, mr := newSvc(t)
	ctx := context.Background()
	ch, _ := s.Send(ctx, "sess", "5511666666666", Options{TTL: 30 * time.Second})
	mr.FastForward(31 * time.Second)
	r, _ := s.Verify(ctx, "sess", "5511666666666", "", ch.code)
	if r.Valid || r.Reason != "not_found" && r.Reason != "expired" {
		t.Fatalf("devia expirar, veio %+v", r)
	}
}

func TestHourlyCap(t *testing.T) {
	s, mr := newSvc(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := s.Send(ctx, "s", "5511555555555", Options{HourlyCap: 3, ResendAfter: time.Second}); err != nil {
			t.Fatalf("envio %d: %v", i, err)
		}
		mr.FastForward(2 * time.Second)
	}
	_, err := s.Send(ctx, "s", "5511555555555", Options{HourlyCap: 3, ResendAfter: time.Second})
	re, ok := err.(*RateError)
	if !ok || re.Reason != "hourly_cap" {
		t.Fatalf("esperava hourly_cap, veio %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
