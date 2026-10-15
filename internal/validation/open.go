// This file holds OpenValidated: the internal consistency check that
// gates every capability on a canonical validation Receipt. It runs no
// validation of its own.

package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// Consistency-contract failures. Callers branch on these with
// errors.Is. Both map to a refusal to proceed; ErrReceiptMismatch is an
// internal error (the receipt does not describe these bytes), while
// ErrReceiptHasErrors is the ordinary "the document is invalid" outcome.
var (
	// ErrReceiptMismatch means the receipt's InputSHA256 does not match
	// the bytes handed to OpenValidated.
	ErrReceiptMismatch = errors.New("validation: receipt input hash does not match the input bytes")
	// ErrReceiptHasErrors means the receipt reports one or more
	// canonical validation errors.
	ErrReceiptHasErrors = errors.New("validation: receipt reports canonical validation errors")
	// ErrReceiptIncomplete means the receipt is missing a field
	// OpenValidated needs to trust it.
	ErrReceiptIncomplete = errors.New("validation: receipt is incomplete")
)

// OpenValidated enforces the library validation boundary. It performs no
// validation itself; it checks that receipt was produced for exactly
// these bytes and reports no errors.
//
// It returns nil only when:
//
//   - receipt.InputSHA256 is a well-formed SHA-256 hex string,
//   - it equals sha256(input), and
//   - receipt.ErrorCount is zero.
//
// Otherwise it returns ErrReceiptIncomplete, ErrReceiptMismatch, or
// ErrReceiptHasErrors. A non-nil return means no capability may run on
// input.
func OpenValidated(input []byte, receipt Receipt) error {
	if len(receipt.InputSHA256) != hex.EncodedLen(sha256.Size) {
		return fmt.Errorf("%w: InputSHA256 is not a SHA-256 hex digest", ErrReceiptIncomplete)
	}
	want, err := hex.DecodeString(receipt.InputSHA256)
	if err != nil {
		return fmt.Errorf("%w: InputSHA256 is not hex: %v", ErrReceiptIncomplete, err)
	}

	got := sha256.Sum256(input)
	if !bytes.Equal(got[:], want) {
		return fmt.Errorf("%w (receipt %s, input %s)",
			ErrReceiptMismatch, receipt.InputSHA256, hex.EncodeToString(got[:]))
	}

	if receipt.ErrorCount != 0 {
		return fmt.Errorf("%w (%d error(s))", ErrReceiptHasErrors, receipt.ErrorCount)
	}
	return nil
}
