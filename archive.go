package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	rardecode "github.com/nwaples/rardecode/v2"
	"github.com/bodgit/sevenzip"
)

// ArchiveFile is a single file within an opened archive.
type ArchiveFile interface {
	Name() string
	Open() (io.ReadCloser, error)
}

// Archive provides access to the files in an opened archive.
type Archive interface {
	io.Closer
	Files() []ArchiveFile // all non-directory entries
}

// openArchive opens the archive at path, dispatching by file extension.
func openArchive(path string) (Archive, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip", ".cbz":
		return openZipArchive(path)
	case ".rar", ".cbr":
		return openRarArchive(path)
	case ".7z", ".cb7":
		return openSevenZipArchive(path)
	case ".tar", ".cbt":
		return openTarArchive(path)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", filepath.Ext(path))
	}
}

// --- ZIP ---

type zipArchive struct{ rc *zip.ReadCloser }

func openZipArchive(path string) (Archive, error) {
	rc, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return &zipArchive{rc: rc}, nil
}

func (a *zipArchive) Close() error { return a.rc.Close() }

func (a *zipArchive) Files() []ArchiveFile {
	out := make([]ArchiveFile, 0, len(a.rc.File))
	for _, f := range a.rc.File {
		if !f.FileInfo().IsDir() {
			out = append(out, zipFile{f})
		}
	}
	return out
}

type zipFile struct{ f *zip.File }

func (f zipFile) Name() string                 { return f.f.Name }
func (f zipFile) Open() (io.ReadCloser, error) { return f.f.Open() }

// --- 7-Zip ---

type sevenzipArchive struct{ rc *sevenzip.ReadCloser }

func openSevenZipArchive(path string) (Archive, error) {
	rc, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return &sevenzipArchive{rc: rc}, nil
}

func (a *sevenzipArchive) Close() error { return a.rc.Close() }

func (a *sevenzipArchive) Files() []ArchiveFile {
	out := make([]ArchiveFile, 0, len(a.rc.File))
	for _, f := range a.rc.File {
		if !f.FileInfo().IsDir() {
			out = append(out, sevenzipFile{f})
		}
	}
	return out
}

type sevenzipFile struct{ f *sevenzip.File }

func (f sevenzipFile) Name() string                 { return f.f.Name }
func (f sevenzipFile) Open() (io.ReadCloser, error) { return f.f.Open() }

// --- TAR ---

type tarEntry struct {
	name         string
	offset, size int64
}

type tarArchive struct {
	f       *os.File
	entries []tarEntry
}

// openTarArchive scans all headers to record data offsets, then keeps the file
// open for O(1) random access via io.SectionReader / ReadAt.
func openTarArchive(path string) (Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(f)
	var entries []tarEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// tar.Reader reads directly from f with no buffering, so SeekCurrent
		// reflects the exact byte offset of this entry's data.
		offset, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			f.Close()
			return nil, err
		}
		entries = append(entries, tarEntry{name: hdr.Name, offset: offset, size: hdr.Size})
		if _, err := io.Copy(io.Discard, tr); err != nil {
			f.Close()
			return nil, err
		}
	}
	return &tarArchive{f: f, entries: entries}, nil
}

func (a *tarArchive) Close() error { return a.f.Close() }

func (a *tarArchive) Files() []ArchiveFile {
	out := make([]ArchiveFile, len(a.entries))
	for i := range a.entries {
		out[i] = &tarFile{f: a.f, e: &a.entries[i]}
	}
	return out
}

type tarFile struct {
	f *os.File
	e *tarEntry
}

func (f *tarFile) Name() string { return f.e.name }
func (f *tarFile) Open() (io.ReadCloser, error) {
	return io.NopCloser(io.NewSectionReader(f.f, f.e.offset, f.e.size)), nil
}

// --- RAR ---

// solidRarCacheLimit caps total cached bytes per solid-RAR archive.
// Eviction uses a sliding-window policy (lowest archIdx first).
const solidRarCacheLimit = 64 << 20 // 64 MiB

type rarArchive struct {
	path  string
	files []ArchiveFile

	// Fields below are only used for solid-file access.
	mu        sync.Mutex
	rc        *rardecode.ReadCloser
	pos       int            // archive-order index of next entry to read via rc
	cache     map[int][]byte // archIdx → decompressed bytes
	cacheSize int
}

func openRarArchive(path string) (Archive, error) {
	list, err := rardecode.List(path)
	if err != nil {
		return nil, err
	}
	ra := &rarArchive{
		path:  path,
		cache: make(map[int][]byte),
	}
	for i, f := range list {
		if f.IsDir {
			continue
		}
		if f.Solid {
			ra.files = append(ra.files, &solidRarEntry{archIdx: i, name: f.Name, arch: ra})
		} else {
			ra.files = append(ra.files, &nonSolidRarEntry{f: f})
		}
	}
	return ra, nil
}

func (a *rarArchive) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rc != nil {
		a.rc.Close()
		a.rc = nil
	}
	a.cache = nil
	return nil
}

func (a *rarArchive) Files() []ArchiveFile { return a.files }

func (a *rarArchive) openRC() error {
	if a.rc != nil {
		a.rc.Close()
	}
	rc, err := rardecode.OpenReader(a.path)
	if err != nil {
		return err
	}
	a.rc = rc
	a.pos = 0
	return nil
}

// readSolid decompresses and returns the bytes for the solid entry at archIdx.
// It advances the shared sequential reader forward, caching everything it
// passes through. On a backward seek it reopens the reader (cache is retained).
func (a *rarArchive) readSolid(archIdx int) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if data, ok := a.cache[archIdx]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	if a.rc == nil || archIdx < a.pos {
		if err := a.openRC(); err != nil {
			return nil, err
		}
	}

	for a.pos <= archIdx {
		hdr, err := a.rc.Next()
		if err != nil {
			return nil, err
		}
		// Read and cache solid files and always read the target.
		// For non-solid / dir entries, Next() drains them internally on the
		// following call (rardecode does io.Copy(Discard) for solid archives).
		if !hdr.IsDir && (hdr.Solid || a.pos == archIdx) {
			data, err := io.ReadAll(a.rc)
			if err != nil {
				return nil, err
			}
			a.cache[a.pos] = data
			a.cacheSize += len(data)
		}
		a.pos++
	}

	// Evict oldest entries (lowest archIdx) while over limit; never evict target.
	for a.cacheSize > solidRarCacheLimit {
		minKey := -1
		for k := range a.cache {
			if k != archIdx && (minKey < 0 || k < minKey) {
				minKey = k
			}
		}
		if minKey < 0 {
			break
		}
		a.cacheSize -= len(a.cache[minKey])
		delete(a.cache, minKey)
	}

	return io.NopCloser(bytes.NewReader(a.cache[archIdx])), nil
}

type nonSolidRarEntry struct{ f *rardecode.File }

func (e *nonSolidRarEntry) Name() string                 { return e.f.Name }
func (e *nonSolidRarEntry) Open() (io.ReadCloser, error) { return e.f.Open() }

type solidRarEntry struct {
	archIdx int
	name    string
	arch    *rarArchive
}

func (e *solidRarEntry) Name() string                 { return e.name }
func (e *solidRarEntry) Open() (io.ReadCloser, error) { return e.arch.readSolid(e.archIdx) }
