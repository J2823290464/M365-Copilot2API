package chathub

import (
	"strings"
	"testing"
)

func TestIsUpstreamBlockedSignal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"exact marker", "<block>no</block>", true},
		{"whitespace padded", "  \n<block>no</block>\n ", true},
		{"uppercase", "<BLOCK>NO</BLOCK>", true},
		{"other payload", "<block>yes</block>", true},
		{"normal answer", "The ERP quote page is at /quote/list.", false},
		{"empty", "", false},
		{"tag without close", "<block>no", false},
		{"long text containing marker", strings.Repeat("x", 200) + "<block>no</block>", false},
	}
	for _, c := range cases {
		if got := IsUpstreamBlockedSignal(c.in); got != c.want {
			t.Fatalf("%s: got %t want %t", c.name, got, c.want)
		}
	}
}
