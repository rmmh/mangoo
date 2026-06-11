package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// syntheticImages returns N deterministic 1×1 PNGs. Image i has a unique color
// derived from i, so round-trip identity checks are meaningful.
func syntheticImages(n int) [][]byte {
	out := make([][]byte, n)
	for i := range n {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, color.RGBA{
			R: uint8(i * 17),
			G: uint8(i * 5),
			B: uint8(255 - i*17),
			A: 255,
		})
		var buf bytes.Buffer
		png.Encode(&buf, img)
		out[i] = buf.Bytes()
	}
	return out
}

// writeImages writes images as NNN.png files into dir and returns their paths.
func writeImages(t *testing.T, dir string, imgs [][]byte) []string {
	t.Helper()
	paths := make([]string, len(imgs))
	for i, data := range imgs {
		p := filepath.Join(dir, fmt.Sprintf("%03d.png", i))
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		paths[i] = p
	}
	return paths
}

func createZip(t *testing.T, imgs [][]byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.cbz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for i, data := range imgs {
		fw, err := w.Create(fmt.Sprintf("%03d.png", i))
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(data)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func createTar(t *testing.T, imgs [][]byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.cbt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	for i, data := range imgs {
		tw.WriteHeader(&tar.Header{
			Name:     fmt.Sprintf("%03d.png", i),
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		})
		tw.Write(data)
	}
	tw.Close()
	return f.Name()
}

func find7z(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"7z", "7za", "7zz"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("7z not found in PATH")
	return ""
}

func create7z(t *testing.T, sevenz string, imgs [][]byte, solid bool) string {
	t.Helper()
	dir := t.TempDir()
	writeImages(t, dir, imgs)
	out := filepath.Join(t.TempDir(), "archive.cb7")
	ms := "-ms=off"
	if solid {
		ms = "-ms=on"
	}
	cmd := exec.Command(sevenz, "a", ms, out, filepath.Join(dir, "*.png"))
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z failed: %v\n%s", err, b)
	}
	return out
}

// testRoundtrip opens the archive at path, filters+sorts images, and verifies
// each file's content matches the corresponding entry in want (by sort order).
func testRoundtrip(t *testing.T, path string, want [][]byte) {
	t.Helper()
	a, err := openArchive(path)
	if err != nil {
		t.Fatalf("openArchive: %v", err)
	}
	defer a.Close()

	images := filterAndSortImages(a.Files())
	if len(images) != len(want) {
		t.Fatalf("got %d images, want %d", len(images), len(want))
	}
	for i, f := range images {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("[%d] Open: %v", i, err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("[%d] ReadAll: %v", i, err)
		}
		if !bytes.Equal(got, want[i]) {
			t.Errorf("[%d] %s: content mismatch (got %d bytes, want %d bytes)", i, f.Name(), len(got), len(want[i]))
		}
	}
}

// testConcurrent simulates the stream.go worker-pool pattern: all images are
// opened concurrently and their content is verified against want.
func testConcurrent(t *testing.T, path string, want [][]byte) {
	t.Helper()
	a, err := openArchive(path)
	if err != nil {
		t.Fatalf("openArchive: %v", err)
	}
	defer a.Close()

	images := filterAndSortImages(a.Files())
	if len(images) != len(want) {
		t.Fatalf("got %d images, want %d", len(images), len(want))
	}

	errs := make([]error, len(images))
	var wg sync.WaitGroup
	for i, f := range images {
		wg.Add(1)
		go func(i int, f ArchiveFile) {
			defer wg.Done()
			rc, err := f.Open()
			if err != nil {
				errs[i] = fmt.Errorf("Open: %w", err)
				return
			}
			got, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				errs[i] = fmt.Errorf("ReadAll: %w", err)
				return
			}
			if !bytes.Equal(got, want[i]) {
				errs[i] = fmt.Errorf("content mismatch (got %d bytes, want %d)", len(got), len(want[i]))
			}
		}(i, f)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("[%d] %s: %v", i, images[i].Name(), err)
		}
	}
}

// testSolidRarReverseAccess exercises the backward-seek path in rarArchive by
// opening a later file, then an earlier one, then advancing again.
func testSolidRarReverseAccess(t *testing.T, path string, want [][]byte) {
	t.Helper()
	a, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	images := filterAndSortImages(a.Files())
	check := func(i int) {
		rc, err := images[i].Open()
		if err != nil {
			t.Fatalf("[%d] Open: %v", i, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(got, want[i]) {
			t.Errorf("[%d] content mismatch", i)
		}
	}
	check(10) // forward pass
	check(0)  // backward: forces reopen
	check(1)  // forward again
	check(10) // should hit cache
}

const nImages = 15

// staticArchiveFile is a minimal ArchiveFile used to test filterAndSortImages.
type staticArchiveFile struct{ name string }

func (f staticArchiveFile) Name() string                 { return f.name }
func (f staticArchiveFile) Open() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(nil)), nil }

func TestFilterAndSortImages(t *testing.T) {
	input := []ArchiveFile{
		staticArchiveFile{"__MACOSX/cover.jpg"},         // MACOSX prefix — excluded
		staticArchiveFile{".hidden.png"},                 // hidden — excluded
		staticArchiveFile{"chapter1/banner.jpg"},         // contains "banner" — excluded
		staticArchiveFile{"metadata.json"},               // non-image ext — excluded
		staticArchiveFile{"chapter1/003.png"},
		staticArchiveFile{"chapter1/001.JPG"},            // uppercase ext — included
		staticArchiveFile{"chapter1/002.jpeg"},
		staticArchiveFile{"cover.webp"},
		staticArchiveFile{"extra.gif"},
		staticArchiveFile{"chapter1/BANNER_wide.png"},    // "banner" case-insensitive — excluded
	}
	got := filterAndSortImages(input)
	want := []string{
		"chapter1/001.JPG",
		"chapter1/002.jpeg",
		"chapter1/003.png",
		"cover.webp",
		"extra.gif",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %v", len(got), len(want), func() []string {
			names := make([]string, len(got))
			for i, f := range got { names[i] = f.Name() }
			return names
		}())
	}
	for i, f := range got {
		if f.Name() != want[i] {
			t.Errorf("[%d] got %q, want %q", i, f.Name(), want[i])
		}
	}
}

// TestArchiveFileMultiOpen verifies that Open() can be called multiple times on
// the same ArchiveFile and returns independent readers each time.
func TestArchiveFileMultiOpen(t *testing.T) {
	imgs := syntheticImages(3)
	for _, create := range []struct {
		name string
		fn   func() string
	}{
		{"zip", func() string { return createZip(t, imgs) }},
		{"tar", func() string { return createTar(t, imgs) }},
		{"rar_solid", func() string { return "testdata/solid.cbr" }},
	} {
		t.Run(create.name, func(t *testing.T) {
			a, err := openArchive(create.fn())
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			images := filterAndSortImages(a.Files())
			f := images[0]
			for range 3 {
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("Open: %v", err)
				}
				got, _ := io.ReadAll(rc)
				rc.Close()
				if !bytes.Equal(got, imgs[0]) {
					t.Errorf("content mismatch on repeated Open()")
				}
			}
		})
	}
}

func TestArchiveZip(t *testing.T) {
	imgs := syntheticImages(nImages)
	path := createZip(t, imgs)
	testRoundtrip(t, path, imgs)
	testConcurrent(t, path, imgs)
}

func TestArchiveTar(t *testing.T) {
	imgs := syntheticImages(nImages)
	path := createTar(t, imgs)
	testRoundtrip(t, path, imgs)
	testConcurrent(t, path, imgs)
}

func TestArchive7zNonSolid(t *testing.T) {
	sz := find7z(t)
	imgs := syntheticImages(nImages)
	path := create7z(t, sz, imgs, false)
	testRoundtrip(t, path, imgs)
	testConcurrent(t, path, imgs)
}

func TestArchive7zSolid(t *testing.T) {
	sz := find7z(t)
	imgs := syntheticImages(nImages)
	path := create7z(t, sz, imgs, true)
	testRoundtrip(t, path, imgs)
	testConcurrent(t, path, imgs)
}

func TestArchiveRarNonSolid(t *testing.T) {
	imgs := syntheticImages(nImages)
	testRoundtrip(t, "testdata/nonsolid.cbr", imgs)
	testConcurrent(t, "testdata/nonsolid.cbr", imgs)
}

func TestArchiveRarSolid(t *testing.T) {
	imgs := syntheticImages(nImages)
	testRoundtrip(t, "testdata/solid.cbr", imgs)
	testConcurrent(t, "testdata/solid.cbr", imgs)
	testSolidRarReverseAccess(t, "testdata/solid.cbr", imgs)
}
