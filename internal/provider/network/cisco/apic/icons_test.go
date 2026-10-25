// Tests for the Cisco APIC model-to-prIcon mapping: a fixed, confirmed
// lookup, never a guessed pattern for a model outside it.

package apic

import "testing"

func TestPrIconForModelConfirmedFamilies(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{"N9K-C9508", "nexus_9300"},
		{"N9K-C93180YC-FX", "nexus_9300"},
		{"N9K-C93108TC-FX", "nexus_9300"},
		{"N1K-model", "nexus_1k"},
		{"N2K-model", "nexus_2k"},
		{"N3K-model", "nexus_3k"},
		{"N4K-model", "nexus_4k"},
		{"N5K-model", "nexus_5k"},
		{"N7K-model", "nexus_7k"},
	}
	for _, tc := range cases {
		if got := prIconForModel(tc.model); got != tc.want {
			t.Errorf("prIconForModel(%q) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

func TestPrIconForModelUnrecognized(t *testing.T) {
	for _, model := range []string{"N6K-model", "N8K-model", "N0K-x", "", "ASR9006", "N9-notk"} {
		if got := prIconForModel(model); got != "" {
			t.Errorf("prIconForModel(%q) = %q, want \"\" (unrecognized, no guess)", model, got)
		}
	}
}
