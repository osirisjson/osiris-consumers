// Package render holds the rendering contract shared by every consumer
// output format.
//
// A Renderer turns a format-neutral capability model into bytes on a
// caller-supplied writer. Every write is checked: the first write error
// is captured and propagated, and no renderer is allowed to swallow it.
// Renderers take a context and check it at array and section boundaries
// so a large document cancels promptly. A renderer never reads the wall
// clock; an artifact time it needs is passed in.
//
// This phase defines only the interface, the checked writer, and the
// destination-aware escaping helpers in the markdown and drawio
// subpackages.
package render

import (
	"context"
	"io"
)

// A Renderer writes one artifact for one capability model.
//
// Implementations must:
//   - write only through w,
//   - return the first error w reports without wrapping it away,
//   - check ctx at loop and section boundaries and return ctx.Err()
//     promptly on cancellation,
//   - be deterministic: identical model and options produce identical
//     bytes, with any timestamp supplied as an input rather than read
//     from the clock.
type Renderer interface {
	Render(ctx context.Context, w io.Writer) error
}

// CheckedWriter wraps an io.Writer so the first write error is recorded
// and every subsequent write is a no-op that returns the same error. It
// lets a renderer write in a straight line and check once at the end,
// while guaranteeing an ignored error cannot pass silently.
type CheckedWriter struct {
	w   io.Writer
	n   int64
	err error
}

// NewCheckedWriter returns a CheckedWriter over w.
func NewCheckedWriter(w io.Writer) *CheckedWriter { return &CheckedWriter{w: w} }

// Write forwards to the underlying writer until the first error, then
// returns that error for every later call without touching the writer.
func (c *CheckedWriter) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	if err != nil {
		c.err = err
		return n, err
	}
	if n < len(p) {
		c.err = io.ErrShortWrite
		return n, io.ErrShortWrite
	}
	return n, nil
}

// WriteString is a convenience for Write([]byte(s)).
func (c *CheckedWriter) WriteString(s string) (int, error) { return c.Write([]byte(s)) }

// Err returns the first write error seen, or nil.
func (c *CheckedWriter) Err() error { return c.err }

// Count returns the number of bytes successfully written.
func (c *CheckedWriter) Count() int64 { return c.n }
