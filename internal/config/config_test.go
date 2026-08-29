package config

import (
	"reflect"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
		{"https://x.com, https://y.com", []string{"https://x.com", "https://y.com"}},
	}
	for _, c := range cases {
		if got := splitCSV(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("WA_T", "true")
	if !envBool("WA_T", false) {
		t.Error(`"true" deveria virar true`)
	}
	t.Setenv("WA_T", "0")
	if envBool("WA_T", true) {
		t.Error(`"0" deveria virar false`)
	}
	t.Setenv("WA_T", "lixo")
	if !envBool("WA_T", true) {
		t.Error("valor invalido deveria usar o default")
	}
	if envBool("WA_MISSING", false) {
		t.Error("var ausente deveria usar o default")
	}
}
