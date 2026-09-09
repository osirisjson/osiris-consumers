// This file implements bounded, exact-byte input reading: a single
// complete JSON value, a hard size ceiling, and a required end of input
// after the value. All failures are typed and happen before any side
// effect.

package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// DefaultMaxInputBytes is the input size ceiling applied when a caller
// passes a non-positive limit to Read. It is deliberately generous for
// a topology snapshot while still bounding memory for an untrusted
// stream.
const DefaultMaxInputBytes int64 = 64 << 20 // 64 MiB

// Input-reading error categories. Callers branch on these with
// errors.Is; the CLI adapter maps every one of them to exit code 2
// (malformed input or operational failure), never exit code 1.
var (
	// ErrOversizeInput means the input exceeded the configured size
	// bound before a complete value was read.
	ErrOversizeInput = errors.New("document: input exceeds the configured size limit")
	// ErrEmptyInput means the input held no JSON value at all.
	ErrEmptyInput = errors.New("document: input is empty please select an OSIRIS JSON file")
	// ErrMalformedJSON means the bytes are not a single well-formed
	// JSON value.
	ErrMalformedJSON = errors.New("document: input is not valid JSON")
	// ErrTrailingData means a complete JSON value was followed by more
	// non-whitespace bytes (for example a second concatenated value).
	ErrTrailingData = errors.New("document: unexpected data after the first JSON value")
)

// Read consumes r and returns the exact bytes of exactly one complete
// JSON value.
//
// maxBytes caps how much input is read; a value of zero or less selects
// DefaultMaxInputBytes. If the input reaches the cap before a complete
// value is available, Read fails with ErrOversizeInput and an error
// message that names the limit and how to raise it.
//
// After the first JSON value, only trailing whitespace is allowed. A
// second value, or any other trailing non-whitespace byte, fails with
// ErrTrailingData. Malformed input fails with ErrMalformedJSON and an
// empty stream fails with ErrEmptyInput. Every failure returns before
// the bytes are used for anything else.
func Read(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxInputBytes
	}

	// Read one byte past the limit so an input exactly at the ceiling
	// is accepted while anything larger is caught.
	limited := io.LimitReader(r, maxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("document: reading input: %w", err)
	}
	if int64(len(buf)) > maxBytes {
		return nil, fmt.Errorf(
			"%w (%d bytes); raise the limit with the size-bound option (bytes) or --max-input-bytes on the CLI",
			ErrOversizeInput, maxBytes)
	}

	if len(bytes.TrimSpace(buf)) == 0 {
		return nil, ErrEmptyInput
	}

	value, rest, err := scanOneJSONValue(buf)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, ErrTrailingData
	}

	// Return the exact value bytes, trimmed of surrounding whitespace
	// only. The hash and the validator see the same content the parser
	// sees.
	out := make([]byte, len(value))
	copy(out, value)
	return out, nil
}
