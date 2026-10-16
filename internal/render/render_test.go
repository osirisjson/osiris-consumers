package render

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestCheckedWriterCountsAndForwards(t *testing.T) {
	var buf bytes.Buffer
	cw := NewCheckedWriter(&buf)
	if _, err := cw.WriteString("hello "); err != nil {
		t.Fatal(err)
	}
	if _, err := cw.Write([]byte("world")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello world" {
		t.Errorf("buf = %q", buf.String())
	}
	if cw.Count() != 11 {
		t.Errorf("Count = %d, want 11", cw.Count())
	}
	if cw.Err() != nil {
		t.Errorf("Err = %v", cw.Err())
	}
}

func TestCheckedWriterStopsAfterFirstError(t *testing.T) {
	boom := errors.New("boom")
	cw := NewCheckedWriter(errWriterAt{after: 4, err: boom})

	_, err := cw.WriteString("12345")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	// Subsequent writes are no-ops returning the same error.
	n, err := cw.WriteString("more")
	if n != 0 || !errors.Is(err, boom) {
		t.Fatalf("after error: n=%d err=%v", n, err)
	}
	if !errors.Is(cw.Err(), boom) {
		t.Fatalf("Err = %v", cw.Err())
	}
}

func TestCheckedWriterShortWrite(t *testing.T) {
	cw := NewCheckedWriter(shortWriterAt{limit: 2})
	if _, err := cw.WriteString("12345"); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("err = %v, want io.ErrShortWrite", err)
	}
}

type errWriterAt struct {
	after int
	err   error
	seen  int
}

func (w errWriterAt) Write(p []byte) (int, error) {
	return 0, w.err
}

type shortWriterAt struct{ limit int }

func (w shortWriterAt) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		return w.limit, nil
	}
	return len(p), nil
}
