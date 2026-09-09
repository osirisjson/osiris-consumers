// This file isolates the single-JSON-value scan used by Read.
// It reports the exact byte span of the first value and whatever bytes
// follow it, so the caller can enforce "one value, then end of input".

package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// scanOneJSONValue decodes the first complete JSON value in buf and
// returns its exact bytes (surrounding whitespace removed) together
// with everything that follows it. A decode failure is reported as
// ErrMalformedJSON; a value-less input as ErrEmptyInput.
func scanOneJSONValue(buf []byte) (value, rest []byte, err error) {
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()

	var raw json.RawMessage
	if decErr := dec.Decode(&raw); decErr != nil {
		if errors.Is(decErr, io.EOF) {
			return nil, nil, ErrEmptyInput
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrMalformedJSON, decErr)
	}

	off := dec.InputOffset()
	return bytes.TrimSpace(buf[:off]), buf[off:], nil
}
