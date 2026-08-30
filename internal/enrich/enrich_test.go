package enrich

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	if New("", "", "", "").Enabled() {
		t.Fatal("sem key não devia estar Enabled")
	}
	if !New("", "k", "", "").Enabled() {
		t.Fatal("com key devia estar Enabled")
	}
	var c *Client
	if c.Enabled() {
		t.Fatal("nil client não devia estar Enabled")
	}
}

func TestTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth = %s", got)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("content-type = %s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "whisper-1") {
			t.Errorf("faltou o campo model no corpo")
		}
		w.Write([]byte(`{"text":"  olá mundo  "}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "k", "", "").Transcribe(context.Background(), []byte("fakeaudio"), "audio/ogg")
	if err != nil {
		t.Fatal(err)
	}
	if got != "olá mundo" {
		t.Fatalf("got %q", got)
	}
}

func TestTranscribeAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "k", "", "").Transcribe(context.Background(), []byte("x"), "audio/ogg")
	if err == nil || !strings.Contains(err.Error(), "bad model") {
		t.Fatalf("err = %v", err)
	}
}

func TestDescribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "data:image/png;base64,") {
			t.Errorf("faltou data URI da imagem: %s", body)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"um gato preto"}}]}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "k", "", "").Describe(context.Background(), []byte("fakeimg"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if got != "um gato preto" {
		t.Fatalf("got %q", got)
	}
}

func TestExtFor(t *testing.T) {
	cases := map[string]string{
		"audio/ogg; codecs=opus": "ogg",
		"audio/mpeg":             "mp3",
		"audio/wav":              "wav",
		"audio/mp4":              "m4a",
		"application/weird":      "ogg",
	}
	for mime, want := range cases {
		if got := extFor(mime); got != want {
			t.Errorf("extFor(%q) = %q, want %q", mime, got, want)
		}
	}
}
