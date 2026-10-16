// Package artifact performs the private, atomic, no-clobber
// finalization of a rendered consumer artifact and records its manifest.
//
// Every write goes to a private temporary file in the target directory,
// flushed, synced, and closed with every error checked, and is only
// then atomically renamed into place. A failure at any step leaves no
// finalized artifact and no success manifest. The destination is opened
// through an os.Root confined to the target directory, so a symlink in
// the path cannot redirect the write; a symlink at the destination
// itself is refused outright, with or without force.
package artifact

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"strings"
)

// File and directory permission modes for finalized artifacts.
const (
	fileMode fs.FileMode = 0o600
	dirMode  fs.FileMode = 0o700
)

// Finalization error categories. Callers branch on these with
// errors.Is. The CLI adapter maps ErrOutputExists,
// ErrSymlinkDestination, ErrNotRegularFile, and ErrPathTraversal to
// exit code 2 (output conflict / usage) and ErrIO to exit code 2
// (operational I/O).
var (
	// ErrPathTraversal means the requested name is not a single safe
	// path element.
	ErrPathTraversal = errors.New("artifact: destination name is not a single path element")
	// ErrSymlinkDestination means the destination path already exists
	// and is a symlink. It is refused regardless of Force.
	ErrSymlinkDestination = errors.New("artifact: destination is a symlink")
	// ErrOutputExists means the destination already exists as a regular
	// file and Force was not set.
	ErrOutputExists = errors.New("artifact: destination already exists")
	// ErrNotRegularFile means the destination exists but is neither a
	// regular file nor a symlink (a directory, device, socket).
	ErrNotRegularFile = errors.New("artifact: destination is not a regular file")
	// ErrIO wraps an operational filesystem failure.
	ErrIO = errors.New("artifact: i/o failure")
)

// A Request describes one artifact finalization.
type Request struct {
	// Dir is the existing target directory. The artifact is created
	// here and never outside it.
	Dir string
	// Name is the final file's base name: exactly one path element,
	// no separators, not "." or "..".
	Name string
	// Force allows overwriting an existing regular file at the
	// destination. It never allows writing through a symlink.
	Force bool

	// Provenance recorded verbatim in the returned Manifest.
	InputSHA256     string
	Capability      string
	ConsumerVersion string
	EngineName      string
	EngineVersion   string
	Profile         string
}

// A Manifest records what was finalized. It is returned only on success;
// a failed Write returns the zero Manifest and a non-nil error.
type Manifest struct {
	Path            string
	Bytes           int64
	ArtifactSHA256  string
	InputSHA256     string
	Capability      string
	ConsumerVersion string
	EngineName      string
	EngineVersion   string
	Profile         string
}

// checkedWriter records the first write error and the running SHA-256
// and byte count, and stops forwarding writes once it has seen an error.
type checkedWriter struct {
	dst  io.Writer
	hash hash.Hash
	n    int64
	err  error
}

func (w *checkedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.dst.Write(p)
	w.n += int64(n)
	_, _ = w.hash.Write(p[:n])
	if err != nil {
		w.err = err
		return n, err
	}
	if n < len(p) {
		w.err = io.ErrShortWrite
		return n, io.ErrShortWrite
	}
	return n, nil
}

// Write renders through produce and finalizes the result atomically.
//
// produce is called exactly once with a writer to the private temporary
// file. Any error it returns, any short write, a cancelled context, or
// failed sync, close, or rename aborts finalization: the temporary file
// is removed and Write returns a non-nil error wrapping the cause. On
// success the destination contains the complete artifact and Write
// returns its Manifest.
func Write(ctx context.Context, req Request, produce func(w io.Writer) error) (Manifest, error) {
	if err := checkName(req.Name); err != nil {
		return Manifest{}, err
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}

	root, err := os.OpenRoot(req.Dir)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: opening target directory: %v", ErrIO, err)
	}
	defer root.Close()

	switch fi, statErr := root.Lstat(req.Name); {
	case statErr == nil:
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			return Manifest{}, fmt.Errorf("%w: %s", ErrSymlinkDestination, req.Name)
		case !fi.Mode().IsRegular():
			return Manifest{}, fmt.Errorf("%w: %s", ErrNotRegularFile, req.Name)
		case !req.Force:
			return Manifest{}, fmt.Errorf("%w: %s (use --force to overwrite)", ErrOutputExists, req.Name)
		}
	case errors.Is(statErr, fs.ErrNotExist):
		// Fresh destination.
	default:
		return Manifest{}, fmt.Errorf("%w: inspecting destination: %v", ErrIO, statErr)
	}

	tmpName, tmp, err := createTemp(root, req.Name)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: creating temporary file: %v", ErrIO, err)
	}
	// From here on every failure path must remove the temporary file.
	cleanup := func() {
		_ = tmp.Close()
		_ = root.Remove(tmpName)
	}

	hasher := sha256.New()
	cw := &checkedWriter{dst: tmp, hash: hasher}

	if err := produce(cw); err != nil {
		cleanup()
		return Manifest{}, fmt.Errorf("rendering artifact: %w", err)
	}
	if cw.err != nil {
		cleanup()
		return Manifest{}, fmt.Errorf("%w: writing artifact: %v", ErrIO, cw.err)
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return Manifest{}, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return Manifest{}, fmt.Errorf("%w: syncing artifact: %v", ErrIO, err)
	}
	if err := tmp.Close(); err != nil {
		_ = root.Remove(tmpName)
		return Manifest{}, fmt.Errorf("%w: closing artifact: %v", ErrIO, err)
	}
	if err := root.Chmod(tmpName, fileMode); err != nil {
		_ = root.Remove(tmpName)
		return Manifest{}, fmt.Errorf("%w: setting artifact permissions: %v", ErrIO, err)
	}
	if err := root.Rename(tmpName, req.Name); err != nil {
		_ = root.Remove(tmpName)
		return Manifest{}, fmt.Errorf("%w: finalizing artifact: %v", ErrIO, err)
	}

	return Manifest{
		Path:            joinDir(req.Dir, req.Name),
		Bytes:           cw.n,
		ArtifactSHA256:  hex.EncodeToString(hasher.Sum(nil)),
		InputSHA256:     req.InputSHA256,
		Capability:      req.Capability,
		ConsumerVersion: req.ConsumerVersion,
		EngineName:      req.EngineName,
		EngineVersion:   req.EngineVersion,
		Profile:         req.Profile,
	}, nil
}

// checkName rejects anything that is not a single safe path element.
func checkName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("%w: %q", ErrPathTraversal, name)
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: %q", ErrPathTraversal, name)
	}
	return nil
}

// createTemp opens a fresh private temporary file in root, named so it
// sits beside the eventual destination in the same directory.
func createTemp(root *os.Root, finalName string) (string, *os.File, error) {
	for attempt := 0; attempt < 10000; attempt++ {
		name := "." + finalName + "." + randomToken() + ".part"
		f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, fileMode)
		if err == nil {
			return name, f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("exhausted temporary name attempts")
}

// randomToken returns 16 hex characters of cryptographic randomness.
func randomToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// joinDir joins a directory and a validated single-element name without
// importing path/filepath semantics into the safety-critical path.
func joinDir(dir, name string) string {
	if dir == "" {
		return name
	}
	if strings.HasSuffix(dir, string(os.PathSeparator)) {
		return dir + name
	}
	return dir + string(os.PathSeparator) + name
}
