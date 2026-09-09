// Tests for bounded exact-byte input reading: size ceiling, single
// value, and required end of input.

package document

import (
	"errors"
	"strings"
	"testing"
)

func TestReadAcceptsOneValue(t *testing.T) {
	in := `  {"version":"1.0.0"}` + "\n\t "
	got, err := Read(strings.NewReader(in), 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != `{"version":"1.0.0"}` {
		t.Fatalf("Read returned %q, want the trimmed value", got)
	}
}

func TestReadRejectsTrailingValue(t *testing.T) {
	in := `{"a":1}{"b":2}`
	_, err := Read(strings.NewReader(in), 0)
	if !errors.Is(err, ErrTrailingData) {
		t.Fatalf("Read error = %v, want ErrTrailingData", err)
	}
}

func TestReadRejectsTrailingGarbage(t *testing.T) {
	in := `{"a":1} not-json-after`
	_, err := Read(strings.NewReader(in), 0)
	if !errors.Is(err, ErrTrailingData) {
		t.Fatalf("Read error = %v, want ErrTrailingData", err)
	}
}

func TestReadRejectsMalformed(t *testing.T) {
	_, err := Read(strings.NewReader(`{"a":`), 0)
	if !errors.Is(err, ErrMalformedJSON) {
		t.Fatalf("Read error = %v, want ErrMalformedJSON", err)
	}
}

func TestReadRejectsEmpty(t *testing.T) {
	for _, in := range []string{"", "   \n\t  "} {
		_, err := Read(strings.NewReader(in), 0)
		if !errors.Is(err, ErrEmptyInput) {
			t.Fatalf("Read(%q) error = %v, want ErrEmptyInput", in, err)
		}
	}
}

func TestReadEnforcesSizeBound(t *testing.T) {
	big := "[" + strings.Repeat("0,", 5000) + "0]" // > 32 bytes
	_, err := Read(strings.NewReader(big), 32)
	if !errors.Is(err, ErrOversizeInput) {
		t.Fatalf("Read error = %v, want ErrOversizeInput", err)
	}
	if !strings.Contains(err.Error(), "raise the limit") {
		t.Fatalf("oversize error should explain how to raise the limit, got %q", err)
	}
}

func TestReadAcceptsInputExactlyAtBound(t *testing.T) {
	in := `{"k":"v"}` // 9 bytes
	got, err := Read(strings.NewReader(in), int64(len(in)))
	if err != nil {
		t.Fatalf("Read at exact bound: %v", err)
	}
	if string(got) != in {
		t.Fatalf("Read returned %q, want %q", got, in)
	}
}
