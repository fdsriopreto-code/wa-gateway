package httpapi

import "testing"

func TestDecodeB64(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantMime string
		wantStr  string
		wantErr  bool
	}{
		{"plain", "aGVsbG8=", "", "hello", false},
		{"data uri", "data:text/plain;base64,aGVsbG8=", "text/plain", "hello", false},
		{"data uri sem base64 marker", "data:image/png,aGVsbG8=", "image/png", "hello", false},
		{"espacos", "  aGVsbG8=  ", "", "hello", false},
		{"invalido", "nao??base64", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, mime, err := decodeB64(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if mime != c.wantMime {
				t.Errorf("mime = %q, want %q", mime, c.wantMime)
			}
			if string(data) != c.wantStr {
				t.Errorf("data = %q, want %q", data, c.wantStr)
			}
		})
	}
}
