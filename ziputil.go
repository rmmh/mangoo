package main

import (
	"archive/zip"
	"path/filepath"
	"sort"
	"strings"
)

var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

func filterAndSortImages(files []*zip.File) []*zip.File {
	var images []*zip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
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
	sort.Slice(images, func(i, j int) bool {
		return images[i].Name < images[j].Name
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
