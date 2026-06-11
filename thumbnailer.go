package main

import (
	"bytes"
	"image"
	stdraw "image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pixiv/go-libwebp/webp"
	xdraw "golang.org/x/image/draw"
)

func runThumbnailer(store *Store, ch <-chan struct{}, stats *Stats) {
	for range ch {
		if err := processThumbnails(store, stats); err != nil {
			slog.Error("thumbnailer failed", "err", err)
		}
	}
}

func processThumbnails(store *Store, stats *Stats) error {
	mhashes, err := store.MhashesWithoutThumbnails()
	if err != nil {
		return err
	}
	if len(mhashes) == 0 {
		return nil
	}
	slog.Info("thumbnailer starting", "count", len(mhashes))
	start := time.Now()
	stats.ThumbBacklog.Store(int64(len(mhashes)))

	var done int
	for chunk := range slices.Chunk(mhashes, 16) {
		var batch []ThumbnailRow
		for _, mhash := range chunk {
			path, err := store.GetFilePathForMhash(mhash)
			if err != nil {
				slog.Debug("thumbnailer: no path", "mhash", mhash, "err", err)
				continue
			}
			data, err := makeThumbnail(path)
			if err != nil {
				slog.Debug("thumbnailer: failed", "mhash", mhash, "path", path, "err", err)
				continue
			}
			if data != nil {
				batch = append(batch, ThumbnailRow{Mhash: mhash, Data: data})
			}
		}
		if len(batch) > 0 {
			if err := store.InsertThumbnails(batch); err != nil {
				slog.Error("thumbnailer: insert failed", "err", err)
			}
		}
		done += len(chunk)
		stats.ThumbBacklog.Store(int64(len(mhashes) - done))
		slog.Debug("thumbnailer progress", "done", done, "total", len(mhashes))
		if done%100 == 0 {
			slog.Info("thumbnailer progress", "done", done, "total", len(mhashes))
		}
	}

	slog.Info("thumbnailer complete", "done", done, "elapsed", time.Since(start).Round(time.Millisecond))
	return nil
}

func makeThumbnail(archivePath string) ([]byte, error) {
	a, err := openArchive(archivePath)
	if err != nil {
		return nil, err
	}
	defer a.Close()

	images := filterAndSortImages(a.Files())
	if len(images) == 0 {
		return nil, nil
	}

	for i := range min(3, len(images)) {
		data, err := thumbFromArchiveFile(images[i])
		if err != nil {
			slog.Debug("thumb decode failed", "file", images[i].Name(), "err", err)
			continue
		}
		return data, nil
	}
	return nil, nil
}

func thumbFromArchiveFile(f ArchiveFile) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	img, err := decodeImage(rc, f.Name())
	if err != nil {
		return nil, err
	}

	resized := resizeFit(img, 400, 400)
	rgba := toRGBA(resized)

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

// decodeImage decodes an image, handling WebP specially since go-libwebp
// does not register an image.Decode handler.
func decodeImage(r io.Reader, filename string) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".webp" {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return webp.DecodeRGBA(data, &webp.DecoderOptions{})
	}
	img, _, err := image.Decode(r)
	return img, err
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	stdraw.Draw(dst, b, img, b.Min, stdraw.Src)
	return dst
}

func resizeCover(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	// Crop the source to the target aspect ratio (center), then scale in one pass.
	var sr image.Rectangle
	if srcW*h > srcH*w { // source wider: crop sides
		cropW := srcH * w / h
		x0 := b.Min.X + (srcW-cropW)/2
		sr = image.Rect(x0, b.Min.Y, x0+cropW, b.Max.Y)
	} else { // source taller: crop top/bottom
		cropH := srcW * h / w
		y0 := b.Min.Y + (srcH-cropH)/2
		sr = image.Rect(b.Min.X, y0, b.Max.X, y0+cropH)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, sr, xdraw.Src, nil)
	return dst
}

func resizeFit(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return img
	}
	scale := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	newW := int(math.Round(float64(w) * scale))
	newH := int(math.Round(float64(h) * scale))
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}
