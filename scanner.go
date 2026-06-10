package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runScanner(store *Store, libraries []string, thumbCh chan<- struct{}) {
	if err := scan(store, libraries, thumbCh); err != nil {
		slog.Error("scan failed", "err", err)
	}
	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := scan(store, libraries, thumbCh); err != nil {
			slog.Error("scan failed", "err", err)
		}
	}
}

func scan(store *Store, libraries []string, thumbCh chan<- struct{}) error {
	slog.Info("scan started")
	start := time.Now()

	known, err := store.AllFilePaths()
	if err != nil {
		return err
	}

	var newCount, updatedCount, skippedCount int
	var batch []UpsertRecord

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := store.UpsertBatch(batch); err != nil {
			return err
		}
		batch = batch[:0]
		select {
		case thumbCh <- struct{}{}:
		default:
		}
		return nil
	}

	for _, lib := range libraries {
		if err := filepath.WalkDir(lib, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				slog.Debug("walk error", "path", path, "err", err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext != ".zip" && ext != ".cbz" {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}
			mtime := info.ModTime().Unix()
			size := info.Size()

			delete(known, path)

			existMtime, existSize, found, err := store.GetFileMtimeSize(path)
			if err != nil {
				return err
			}
			if found && existMtime == mtime && existSize == size {
				skippedCount++
				if skippedCount%100 == 0 {
					slog.Info("scan progress", "skipped", skippedCount)
				}
				return nil
			}

			mhash, err := computeMHash(path)
			if err != nil {
				slog.Debug("hash failed", "path", path, "err", err)
				return nil
			}

			pageCount, fileTags, err := inspectZip(path)
			if err != nil {
				slog.Debug("inspect failed", "path", path, "err", err)
				return nil
			}

			title := deriveTitle(path)
			metaJSON, err := buildMetadataJSON(pageCount, fileTags)
			if err != nil {
				return err
			}

			batch = append(batch, UpsertRecord{
				Path: path, Mhash: mhash, Title: title,
				MetadataJSON: metaJSON, Mtime: mtime, Size: size,
			})

			if found {
				updatedCount++
				slog.Debug("updated", "path", path, "mhash", mhash)
				if updatedCount%100 == 0 {
					slog.Info("scan progress", "updated", updatedCount)
				}
			} else {
				newCount++
				slog.Debug("added", "path", path, "mhash", mhash)
				if newCount%100 == 0 {
					slog.Info("scan progress", "new", newCount)
				}
			}

			if len(batch) >= 16 {
				return flush()
			}
			return nil
		}); err != nil {
			slog.Error("walk error", "lib", lib, "err", err)
		}
	}

	if err := flush(); err != nil {
		return err
	}

	var missing []string
	for p := range known {
		missing = append(missing, p)
	}
	if len(missing) > 0 {
		slog.Info("removing missing files", "count", len(missing))
		if err := store.DeleteFiles(missing); err != nil {
			return err
		}
	}
	if err := store.PruneOrphanManga(); err != nil {
		return err
	}
	if err := store.PruneOrphanThumbnails(); err != nil {
		return err
	}

	slog.Info("scan complete",
		"new", newCount,
		"updated", updatedCount,
		"skipped", skippedCount,
		"removed", len(missing),
		"elapsed", time.Since(start).Round(time.Millisecond),
	)

	select {
	case thumbCh <- struct{}{}:
	default:
	}
	return nil
}

type zipMetadata struct {
	Tags []Tag `json:"tags"`
}

func inspectZip(path string) (pageCount int, tags []Tag, err error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0, nil, err
	}
	defer r.Close()

	images := filterAndSortImages(r.File)
	pageCount = len(images)

	// look for metadata.json in the zip
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Base(f.Name), "metadata.json") {
			rc, err := f.Open()
			if err != nil {
				break
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				break
			}
			var meta zipMetadata
			if err := json.Unmarshal(data, &meta); err == nil {
				tags = meta.Tags
			}
			break
		}
	}
	return pageCount, tags, nil
}

func deriveTitle(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return strings.TrimSpace(base)
}

func buildMetadataJSON(pageCount int, tags []Tag) (string, error) {
	m := struct {
		PageCount int   `json:"page_count"`
		Tags      []Tag `json:"tags"`
	}{
		PageCount: pageCount,
		Tags:      tags,
	}
	if m.Tags == nil {
		m.Tags = []Tag{}
	}
	b, err := json.Marshal(m)
	return string(b), err
}
