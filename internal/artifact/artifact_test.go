// Tests for private, atomic, no-clobber artifact finalization: success
// path and manifest, overwrite policy, symlink refusal, name safety,
// and the failure paths that must leave nothing behind.

package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func baseReq(dir, name string) Request {
	return Request{
		Dir: dir, Name: name,
		InputSHA256: "deadbeef", Capability: "report",
		ConsumerVersion: "test", EngineName: "@osirisjson/cli",
		EngineVersion: "0.1.0", Profile: "default",
	}
}

func writeString(s string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	}
}

func TestWriteFinalizesAtomicallyWithManifest(t *testing.T) {
	dir := t.TempDir()
	const body = "# report\n\nbody\n"

	m, err := Write(context.Background(), baseReq(dir, "out.md"), writeString(body))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "out.md"))
	if err != nil {
		t.Fatalf("reading artifact: %v", err)
	}
	if string(got) != body {
		t.Errorf("artifact body = %q", got)
	}
	sum := sha256.Sum256([]byte(body))
	if m.ArtifactSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("manifest ArtifactSHA256 = %q", m.ArtifactSHA256)
	}
	if m.Bytes != int64(len(body)) {
		t.Errorf("manifest Bytes = %d, want %d", m.Bytes, len(body))
	}
	if m.Path != filepath.Join(dir, "out.md") {
		t.Errorf("manifest Path = %q", m.Path)
	}
	if m.Capability != "report" || m.EngineVersion != "0.1.0" || m.InputSHA256 != "deadbeef" {
		t.Errorf("manifest provenance not carried: %+v", m)
	}

	// No leftover temporary files.
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("directory has %d entries, want just the artifact", len(entries))
	}
}

func TestWriteCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not meaningful on Windows")
	}
	dir := t.TempDir()
	if _, err := Write(context.Background(), baseReq(dir, "p.md"), writeString("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dir, "p.md"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestWriteRefusesExistingRegularFileWithoutForce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.md")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Write(context.Background(), baseReq(dir, "exists.md"), writeString("new"))
	if !errors.Is(err, ErrOutputExists) {
		t.Fatalf("error = %v, want ErrOutputExists", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Errorf("existing file was modified")
	}
}

func TestWriteForceOverwritesRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.md")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := baseReq(dir, "exists.md")
	req.Force = true
	if _, err := Write(context.Background(), req, writeString("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("force did not overwrite: %q", b)
	}
}

func TestWriteRefusesSymlinkDestinationEvenWithForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is privileged on Windows")
	}
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.md")
	if err := os.WriteFile(realFile, []byte("real"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatal(err)
	}

	req := baseReq(dir, "link.md")
	req.Force = true
	_, err := Write(context.Background(), req, writeString("hijack"))
	if !errors.Is(err, ErrSymlinkDestination) {
		t.Fatalf("error = %v, want ErrSymlinkDestination", err)
	}
	if b, _ := os.ReadFile(realFile); string(b) != "real" {
		t.Errorf("write followed the symlink: target now %q", b)
	}
}

func TestWriteRejectsUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "sub/../x", string([]byte{'x', 0})} {
		if _, err := Write(context.Background(), baseReq(dir, name), writeString("x")); !errors.Is(err, ErrPathTraversal) {
			t.Errorf("name %q: error = %v, want ErrPathTraversal", name, err)
		}
	}
}

func TestWriteProduceErrorLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	sentinel := errors.New("render blew up")
	_, err := Write(context.Background(), baseReq(dir, "out.md"), func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want the produce error", err)
	}
	assertEmptyDir(t, dir)
}

func TestWritePermissionDeniedIsIOError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory mode enforcement not reliable here")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "ro")
	if err := os.Mkdir(sub, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	_, err := Write(context.Background(), baseReq(sub, "out.md"), writeString("x"))
	if !errors.Is(err, ErrIO) {
		t.Fatalf("error = %v, want ErrIO", err)
	}
}

// TestCheckedWriterSurfacesSinkError exercises the internal checked
// writer directly: a failing sink error must propagate and stick.
func TestCheckedWriterSurfacesSinkError(t *testing.T) {
	boom := errors.New("sink failure")
	cw := &checkedWriter{dst: failingWriter{err: boom}, hash: sha256.New()}
	if _, err := cw.Write([]byte("data")); !errors.Is(err, boom) {
		t.Fatalf("first write err = %v, want the sink error", err)
	}
	if _, err := cw.Write([]byte("more")); !errors.Is(err, boom) {
		t.Fatalf("second write err = %v, want the sticky sink error", err)
	}
	if cw.err == nil {
		t.Fatal("checkedWriter did not retain the error")
	}
}

func TestWriteMissingDirectoryIsIOError(t *testing.T) {
	_, err := Write(context.Background(), baseReq(filepath.Join(t.TempDir(), "no-such-dir"), "out.md"), writeString("x"))
	if !errors.Is(err, ErrIO) {
		t.Fatalf("error = %v, want ErrIO", err)
	}
}

func TestWriteAcceptsDirWithTrailingSeparator(t *testing.T) {
	dir := t.TempDir() + string(os.PathSeparator)
	m, err := Write(context.Background(), baseReq(dir, "out.md"), writeString("body"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(m.Path); err != nil {
		t.Errorf("artifact not at manifest path %q: %v", m.Path, err)
	}
}

func TestWriteRefusesDirectoryDestination(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Write(context.Background(), baseReq(dir, "adir"), writeString("x"))
	if !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("error = %v, want ErrNotRegularFile", err)
	}
}

func TestCheckedWriterFlagsShortWrite(t *testing.T) {
	cw := &checkedWriter{dst: failingWriter{n: 2}, hash: sha256.New()}
	if _, err := cw.Write([]byte("12345")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("err = %v, want io.ErrShortWrite", err)
	}
}

type failingWriter struct {
	err error
	n   int
}

func (f failingWriter) Write(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.n < len(p) {
		return f.n, nil
	}
	return len(p), nil
}

func TestWriteHonoursCancelledContext(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Write(ctx, baseReq(dir, "out.md"), writeString("x"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	assertEmptyDir(t, dir)
}

func TestWriteCancelDuringProduceLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	_, err := Write(ctx, baseReq(dir, "out.md"), func(w io.Writer) error {
		_, _ = io.WriteString(w, "half")
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	assertEmptyDir(t, dir)
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory not empty after failure: %s", strings.Join(names, ", "))
	}
}
