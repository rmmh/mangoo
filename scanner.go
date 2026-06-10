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

const slowThreshold = time.Second

func logSlow(label, path string, start time.Time) {
	if elapsed := time.Since(start); elapsed > slowThreshold {
		slog.Info("slow "+label, "path", path, "elapsed", elapsed.Round(time.Millisecond))
	}
}

func runScanner(store *Store, libraries []string, thumbCh chan<- struct{}, rescanCh <-chan struct{}, stats *Stats) {
	if err := scan(store, libraries, thumbCh, stats); err != nil {
		slog.Error("scan failed", "err", err)
	}
	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-rescanCh:
		}
		if err := scan(store, libraries, thumbCh, stats); err != nil {
			slog.Error("scan failed", "err", err)
		}
	}
}

func scan(store *Store, libraries []string, thumbCh chan<- struct{}, stats *Stats) error {
	slog.Info("scan started")
	start := time.Now()

	known, err := store.AllFilePaths()
	if err != nil {
		return err
	}

	var newCount, updatedCount, skippedCount int
	var batch []UpsertRecord
	var lastSkipLog time.Time

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
		var curDir string
		var dirStart time.Time

		if err := filepath.WalkDir(lib, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				slog.Debug("walk error", "path", path, "err", err)
				return nil
			}
			if d.IsDir() {
				if curDir != "" {
					if elapsed := time.Since(dirStart); elapsed > slowThreshold {
						slog.Info("slow directory scan", "dir", curDir, "elapsed", elapsed.Round(time.Millisecond))
					}
				}
				curDir = path
				dirStart = time.Now()
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
			stats.FilesScanned.Add(1)

			existMtime, existSize, found, err := store.GetFileMtimeSize(path)
			if err != nil {
				return err
			}
			if found && existMtime == mtime && existSize == size {
				skippedCount++
				if time.Since(lastSkipLog) >= 2*time.Second {
					slog.Info("scan progress", "skipped", skippedCount)
					lastSkipLog = time.Now()
				}
				return nil
			}

			fileStart := time.Now()

			t := time.Now()
			mhash, err := computeMHash(path)
			if err != nil {
				slog.Debug("hash failed", "path", path, "err", err)
				return nil
			}
			logSlow("hash", path, t)

			t = time.Now()
			pageCount, fileTags, err := inspectZip(path)
			if err != nil {
				slog.Debug("inspect failed", "path", path, "err", err)
				return nil
			}
			logSlow("zip inspect", path, t)

			title := deriveTitle(path)
			metaJSON, err := buildMetadataJSON(pageCount, fileTags)
			if err != nil {
				return err
			}
			logSlow("file", path, fileStart)

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

		if curDir != "" {
			if elapsed := time.Since(dirStart); elapsed > slowThreshold {
				slog.Info("slow directory scan", "dir", curDir, "elapsed", elapsed.Round(time.Millisecond))
			}
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

	stats.FilesScanned.Store(0)
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

func inspectZip(path string) (pageCount int, tags []Tag, err error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0, nil, err
	}
	defer r.Close()

	images := filterAndSortImages(r.File)
	pageCount = len(images)

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
			var meta metadataJSON
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

