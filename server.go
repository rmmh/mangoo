package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "embed"
)

// 42 is 2*3*7 => fits 2, 3, 6, or 7 columns without ragged items
const kPerPage = 42

//go:embed frontend/index.html
var indexHTML []byte

type server struct {
	store       *Store
	cache       *zipCache
	rescanCh    chan<- struct{}
	thumbnailer bool
}

func runServer(cfg *Config, store *Store, rescanCh chan<- struct{}) {
	s := &server{store: store, cache: newZipCache(), rescanCh: rescanCh, thumbnailer: cfg.Thumbnailer}
	mux := http.NewServeMux()

	// static assets (embedded frontend/dist/)
	staticSub, err := fs.Sub(staticFS, "frontend/dist")
	if err != nil {
		slog.Error("static sub failed", "err", err)
	} else {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	}

	// API
	mux.HandleFunc("GET /api/list", s.handleAPIList)
	mux.HandleFunc("GET /api/manga/{mhash}", s.handleAPIManga)
	mux.HandleFunc("GET /api/similar/{mhash}", s.handleAPISimilar)
	mux.HandleFunc("GET /api/search", s.handleAPISearch)
	mux.HandleFunc("GET /api/random", s.handleAPIRandom)
	mux.HandleFunc("POST /api/rescan", s.handleAPIRescan)
	mux.HandleFunc("GET /thumb/{mhash}", s.handleThumb)
	mux.HandleFunc("GET /g/{mhash}/img/{n}", s.handleImage)
	mux.HandleFunc("/", s.handleSPA)

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("server listening", "addr", addr)
	if err := http.ListenAndServe(addr, logged(mux)); err != nil {
		slog.Error("server error", "err", err)
	}
}

func (s *server) handleAPIList(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	sortBy := queryString(r, "sort", "mtime")
	if sortBy != "title" {
		sortBy = "mtime"
	}

	items, total, err := s.store.ListManga(page, kPerPage, sortBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []MangaListItem{}
	}
	writeJSON(w, map[string]any{
		"manga":    items,
		"total":    total,
		"page":     page,
		"per_page": kPerPage,
	})
}

func (s *server) handleAPIManga(w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	d, err := s.store.GetManga(mhash)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, d)
}

func (s *server) handleAPISimilar(w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	items, err := s.store.SimilarManga(mhash, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []MangaListItem{}
	}
	writeJSON(w, items)
}

func (s *server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := queryInt(r, "page", 1)
	sortBy := queryString(r, "sort", "mtime")
	if sortBy != "title" {
		sortBy = "mtime"
	}
	slog.Debug("search", "q", q, "fts", buildFTSQuery(q))

	items, total, err := s.store.Search(q, page, kPerPage, sortBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []MangaListItem{}
	}
	writeJSON(w, map[string]any{
		"manga":    items,
		"total":    total,
		"page":     page,
		"per_page": kPerPage,
	})
}

func (s *server) handleAPIRescan(w http.ResponseWriter, r *http.Request) {
	select {
	case s.rescanCh <- struct{}{}:
	default:
	}
	writeJSON(w, map[string]string{"status": "queued"})
}

func (s *server) handleAPIRandom(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	mhash, err := s.store.RandomManga(q)
	if err != nil {
		writeError(w, http.StatusNotFound, "no results")
		return
	}
	writeJSON(w, map[string]string{"mhash": mhash})
}

func (s *server) handleSPA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(indexHTML)
}

func (s *server) handleThumb(w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	data, err := s.store.GetThumbnail(mhash)
	if err != nil {
		path, perr := s.store.GetFilePathForMhash(mhash)
		if perr != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		data, err = makeThumbnail(path)
		if err != nil || data == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if s.thumbnailer {
			_ = s.store.InsertThumbnails([]ThumbnailRow{{Mhash: mhash, Data: data}})
		}
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(data)
}

func (s *server) handleImage(w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	nStr := r.PathValue("n")
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 1 {
		http.Error(w, "invalid page", http.StatusBadRequest)
		return
	}

	var maxW, maxH int
	if sv := r.URL.Query().Get("w"); sv != "" {
		maxW, _ = strconv.Atoi(sv)
	}
	if sv := r.URL.Query().Get("h"); sv != "" {
		maxH, _ = strconv.Atoi(sv)
	}

	path, err := s.store.GetFilePathForMhash(mhash)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.cache.serveImage(w, mhash, path, n, maxW, maxH)
}

// --- middleware ---

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rw, r)
		slog.Debug("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"elapsed", time.Since(start).Round(time.Microsecond),
		)
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func queryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return def
	}
	return v
}

func queryString(r *http.Request, key, def string) string {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	return s
}
