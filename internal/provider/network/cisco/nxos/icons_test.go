package nxos

import "testing"

func TestPrIconForModel(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"N9K-C93180YC-FX", "nexus_9300"},
		{"N7K-C7010", "nexus_7k"},
		{"N5K-C5548UP", "nexus_5k"},
		{"n9k-lowercase", "nexus_9300"},
		{"", ""},
		{"WS-C3850-24T-E", ""}, // a Catalyst model, not a Nexus one.
		{"N", ""},
	}
	for _, c := range cases {
		if got := prIconForModel(c.model); got != c.want {
			t.Errorf("prIconForModel(%q) = %q, want %q", c.model, got, c.want)
		}
	}
}
