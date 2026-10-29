// This file holds OpenValidated, the library validation boundary: exact
// bytes plus a receipt in, a read-only validated view out, and a hard
// refusal on any inconsistency.

package consumer

import (
	"errors"
	"fmt"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/validation"
)

// A Validated is a read-only, receipt-checked view of one document. It
// is the value the capability functions build their models from. Every
// method is safe for concurrent use; the value is immutable once
// returned.
type Validated struct {
	doc     *document.Document
	receipt ValidationReceipt
}

// OpenValidated is the library validation boundary. It runs no
// validation of its own: it checks that receipt was produced for
// exactly these bytes and reports no errors, then parses the bytes into
// a read-only view.
//
// It returns ErrReceiptMismatch when the receipt does not describe
// input or is incomplete, ErrValidationFailed when the receipt reports
// canonical errors, ErrUnsupportedOSIRISJSON when the document declares
// a non-1 major version, and ErrInvalidInput when the bytes are not a
// parseable OSIRIS JSON object.
// A non-nil return means no capability may run.
func OpenValidated(input []byte, receipt ValidationReceipt) (*Validated, error) {
	if err := validation.OpenValidated(input, receipt); err != nil {
		switch {
		case errors.Is(err, validation.ErrReceiptHasErrors):
			return nil, fmt.Errorf("%w: %w", ErrValidationFailed, err)
		default:
			return nil, fmt.Errorf("%w: %w", ErrReceiptMismatch, err)
		}
	}

	doc, err := document.Parse(input)
	if err != nil {
		if errors.Is(err, document.ErrUnsupportedMajor) {
			return nil, fmt.Errorf("%w: %w", ErrUnsupportedOSIRISJSON, err)
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return &Validated{doc: doc, receipt: receipt}, nil
}

// InputSHA256 returns the lower-case hex SHA-256 of the exact document
// bytes.
func (v *Validated) InputSHA256() string { return v.doc.SHA256() }

// Receipt returns the canonical validation receipt this view was opened
// with.
func (v *Validated) Receipt() ValidationReceipt { return v.receipt }
