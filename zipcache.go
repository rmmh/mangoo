package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/pixiv/go-libwebp/webp"
)

const cacheCapacity = 8

type zipCache struct {
	mu      sync.Mutex
	cap     int
	lru     []string // mhash order, index 0 = most recent
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	mu     sync.Mutex
	reader *zip.ReadCloser
	images []*zip.File
}

func newZipCache() *zipCache {
	return &zipCache{
		cap:     cacheCapacity,
		entries: make(map[string]*cacheEntry),
	}
}

func (c *zipCache) get(mhash, path string) (*cacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[mhash]; ok {
		c.promote(mhash)
		return e, nil
	}

	// evict LRU if at capacity
	if len(c.lru) >= c.cap {
		oldest := c.lru[len(c.lru)-1]
		c.lru = c.lru[:len(c.lru)-1]
		evicted := c.entries[oldest]
		delete(c.entries, oldest)
		// close after releasing c.mu (we hold it now but close is last thing we do)
		go func() {
			evicted.mu.Lock()
			evicted.reader.Close()
			evicted.mu.Unlock()
		}()
	}

	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip %s: %w", path, err)
	}
	e := &cacheEntry{
		reader: r,
		images: filterAndSortImages(r.File),
	}
	c.entries[mhash] = e
	c.lru = append([]string{mhash}, c.lru...)
	return e, nil
}

func (c *zipCache) promote(mhash string) {
	for i, h := range c.lru {
		if h == mhash {
			c.lru = append(c.lru[:i], c.lru[i+1:]...)
			break
		}
	}
	c.lru = append([]string{mhash}, c.lru...)
}

func (c *zipCache) serveImage(w http.ResponseWriter, mhash, path string, n, maxW, maxH int) {
	e, err := c.get(mhash, path)
	if err != nil {
		http.Error(w, "could not open archive", http.StatusInternalServerError)
		return
	}

	if n < 1 || n > len(e.images) {
		http.Error(w, "page out of range", http.StatusNotFound)
		return
	}
	f := e.images[n-1]

	e.mu.Lock()
	rc, err := f.Open()
	var data []byte
	if err == nil {
		data, err = io.ReadAll(rc)
		rc.Close()
	}
	e.mu.Unlock()

	if err != nil {
		http.Error(w, "could not read image", http.StatusInternalServerError)
		return
	}

	// Skip resize+transcode when the source is small and the target dims are loose
	// (i.e. the image is unlikely to exceed them), so we avoid pointless work.
	smallSource := len(data) < 500_000
	looseDims := (maxW <= 0 || maxW > 700) && (maxH <= 0 || maxH > 700)
	if (maxW > 0 || maxH > 0) && !(smallSource && looseDims) {
		img, err := decodeImage(bytes.NewReader(data), f.Name)
		if err != nil {
			http.Error(w, "could not decode image", http.StatusInternalServerError)
			return
		}
		if maxW == 0 {
			maxW = math.MaxInt
		}
		if maxH == 0 {
			maxH = math.MaxInt
		}
		resized := resizeFit(img, maxW, maxH)
		rgba := toRGBA(resized)
		cfg, err := webp.ConfigPreset(webp.PresetDefault, 90)
		if err != nil {
			http.Error(w, "could not configure encoder", http.StatusInternalServerError)
			return
		}
		var buf bytes.Buffer
		if err := webp.EncodeRGBA(&buf, rgba, cfg); err != nil {
			http.Error(w, "could not encode image", http.StatusInternalServerError)
			return
		}
		data = buf.Bytes()
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(data)
		return
	}

	w.Header().Set("Content-Type", imageMIME(f.Name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}
