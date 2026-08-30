package secret

import "testing"

func TestBoxRoundTrip(t *testing.T) {
	b, err := New("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Enabled() {
		t.Fatal("deveria estar ligado")
	}
	sealed := b.Seal("s3gr3d0")
	if sealed == "s3gr3d0" || sealed[:7] != "enc:v1:" {
		t.Fatalf("não cifrou: %q", sealed)
	}
	if b.Seal(sealed) != sealed {
		t.Fatal("Seal deveria ser idempotente sobre valor já cifrado")
	}
	if got := b.Open(sealed); got != "s3gr3d0" {
		t.Fatalf("Open = %q", got)
	}
	if b.Open("texto-puro") != "texto-puro" {
		t.Fatal("Open não deveria mexer em texto puro")
	}
}

func TestBoxPassthrough(t *testing.T) {
	b, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if b.Enabled() {
		t.Fatal("sem chave = desligado")
	}
	if b.Seal("x") != "x" || b.Open("enc:v1:zzz") != "enc:v1:zzz" {
		t.Fatal("passthrough deveria devolver igual")
	}
}

func TestBoxBadKey(t *testing.T) {
	if _, err := New("abc"); err == nil {
		t.Fatal("chave curta deveria falhar")
	}
}
