// Tests for the OpenValidated consistency contract: hash must match and
// the error count must be zero.

package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func hashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestOpenValidatedAcceptsMatchingCleanReceipt(t *testing.T) {
	in := []byte(`{"version":"1.0.0"}`)
	r := Receipt{InputSHA256: hashOf(in), ErrorCount: 0}
	if err := OpenValidated(in, r); err != nil {
		t.Fatalf("OpenValidated: %v", err)
	}
}

func TestOpenValidatedRejectsHashMismatch(t *testing.T) {
	in := []byte(`{"version":"1.0.0"}`)
	r := Receipt{InputSHA256: hashOf([]byte("different bytes")), ErrorCount: 0}
	if err := OpenValidated(in, r); !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("error = %v, want ErrReceiptMismatch", err)
	}
}

func TestOpenValidatedRejectsErrorBearingReceipt(t *testing.T) {
	in := []byte(`{"version":"1.0.0"}`)
	r := Receipt{InputSHA256: hashOf(in), ErrorCount: 3}
	if err := OpenValidated(in, r); !errors.Is(err, ErrReceiptHasErrors) {
		t.Fatalf("error = %v, want ErrReceiptHasErrors", err)
	}
}

func TestOpenValidatedRejectsIncompleteReceipt(t *testing.T) {
	in := []byte(`{}`)
	for _, r := range []Receipt{
		{},
		{InputSHA256: "not-hex"},
		{InputSHA256: "abcd"},
	} {
		if err := OpenValidated(in, r); !errors.Is(err, ErrReceiptIncomplete) {
			t.Fatalf("receipt %+v: error = %v, want ErrReceiptIncomplete", r, err)
		}
	}
}

func TestOpenValidatedZeroReceiptNeverAccepted(t *testing.T) {
	if err := OpenValidated(nil, Receipt{}); err == nil {
		t.Fatal("zero receipt was accepted")
	}
}
