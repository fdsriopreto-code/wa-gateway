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
	good := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	if _, err := New(good, "zzz"); err == nil {
		t.Fatal("SECRET_KEY_OLD inválida deveria falhar")
	}
}

func TestBoxRotation(t *testing.T) {
	k1 := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	k2 := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	old, _ := New(k1)
	sealedWithK1 := old.Seal("segredo")

	// rotaciona: primária = k2, k1 vira só-leitura
	rot, err := New(k2, k1)
	if err != nil {
		t.Fatal(err)
	}
	if got := rot.Open(sealedWithK1); got != "segredo" {
		t.Fatalf("não abriu com chave antiga: %q", got)
	}
	// novo valor sai com k2 e NÃO abre só com k1
	sealedWithK2 := rot.Seal("novo")
	if only1, _ := New(k1); only1.Open(sealedWithK2) == "novo" {
		t.Fatal("k1 não deveria abrir o que k2 cifrou")
	}
}
