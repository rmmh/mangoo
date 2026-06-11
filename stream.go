package main

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"runtime"
	"strconv"

	"github.com/pixiv/go-libwebp/webp"
)

func (s *server) handleThumbStream(w http.ResponseWriter, r *http.Request) {
	mhash := r.URL.Query().Get("m")
	offset, _ := strconv.Atoi(r.URL.Query().Get("o"))
	thumbW, _ := strconv.Atoi(r.URL.Query().Get("w"))
	thumbH, _ := strconv.Atoi(r.URL.Query().Get("h"))
	if thumbW <= 0 {
		thumbW = 160
	}
	if thumbH <= 0 {
		thumbH = 213
	}

	path, err := s.store.GetFilePathForMhash(mhash)
	if err != nil {
		writeError(w, 404, "not found")
		return
	}

	zr, err := openArchive(path)
	if err != nil {
		writeError(w, 500, "cannot open archive")
		return
	}
	defer zr.Close()

	images := filterAndSortImages(zr.Files())
	if offset >= len(images) {
		w.WriteHeader(200)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)

	flusher, canFlush := w.(http.Flusher)
	ctx := r.Context()

	var countHdr [4]byte
	binary.BigEndian.PutUint32(countHdr[:], uint32(len(images)-offset))
	w.Write(countHdr[:])
	if canFlush {
		flusher.Flush()
	}

	// Worker pool: NumCPU/2 goroutines encode thumbs in parallel; results are
	// collected in per-image channels so they can be flushed in page order.
	total := len(images) - offset
	chs := make([]chan []byte, total)
	for i := range chs {
		chs[i] = make(chan []byte, 1)
	}

	work := make(chan int)
	for range max(1, runtime.NumCPU()/2) {
		go func() {
			for i := range work {
				data, _ := pageThumb(images[offset+i], thumbW, thumbH)
				chs[i] <- data
			}
		}()
	}
	go func() {
		defer close(work)
		for i := range total {
			select {
			case work <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	for _, ch := range chs {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			if data == nil {
				continue
			}
			var hdr [4]byte
			binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
			w.Write(hdr[:])
			w.Write(data)
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

func pageThumb(f ArchiveFile, maxW, maxH int) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	img, err := decodeImage(rc, f.Name())
	if err != nil {
		return nil, err
	}

	img = resizeCover(img, maxW, maxH)
	rgba := toRGBA(img)

	cfg, err := webp.ConfigPreset(webp.PresetDefault, 80)
	if err != nil {
		return nil, err
	}
	cfg.SetMethod(4)

	var buf bytes.Buffer
	if err := webp.EncodeRGBA(&buf, rgba, cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
