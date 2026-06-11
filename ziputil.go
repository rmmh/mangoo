package main

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"
)

var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

func filterAndSortImages(files []ArchiveFile) []ArchiveFile {
	var images []ArchiveFile
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		base := filepath.Base(name)
		if strings.HasPrefix(base, ".") {
			continue
		}
		if strings.Contains(strings.ToLower(base), "banner") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(base))
		if imageExts[ext] {
			images = append(images, f)
		}
	}
	slices.SortFunc(images, func(a, b ArchiveFile) int {
		return cmp.Compare(a.Name(), b.Name())
	})
	return images
}

func imageMIME(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
