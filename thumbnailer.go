package main

import (
	"archive/zip"
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

func runThumbnailer(store *Store, ch <-chan struct{}) {
	for range ch {
		if err := processThumbnails(store); err != nil {
			slog.Error("thumbnailer failed", "err", err)
		}
	}
}

func processThumbnails(store *Store) error {
	mhashes, err := store.MhashesWithoutThumbnails()
	if err != nil {
		return err
	}
	if len(mhashes) == 0 {
		return nil
	}
	slog.Info("thumbnailer starting", "count", len(mhashes))
	start := time.Now()

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
		slog.Debug("thumbnailer progress", "done", done, "total", len(mhashes))
		if done%100 == 0 {
			slog.Info("thumbnailer progress", "done", done, "total", len(mhashes))
		}
	}

	slog.Info("thumbnailer complete", "done", done, "elapsed", time.Since(start).Round(time.Millisecond))
	return nil
}

func makeThumbnail(zipPath string) ([]byte, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	images := filterAndSortImages(r.File)
	if len(images) == 0 {
		return nil, nil
	}

	for i := range min(3, len(images)) {
		data, err := thumbFromZipFile(images[i])
		if err != nil {
			slog.Debug("thumb decode failed", "file", images[i].Name, "err", err)
			continue
		}
		return data, nil
	}
	return nil, nil
}

func thumbFromZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	img, err := decodeImage(rc, f.Name)
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
	xdraw.BiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}
