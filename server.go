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

//go:embed frontend/index.html
var indexHTML []byte

func runServer(cfg *Config, store *Store) {
	cache := newZipCache()
	mux := http.NewServeMux()

	// static assets (embedded frontend/dist/)
	staticSub, err := fs.Sub(staticFS, "frontend/dist")
	if err != nil {
		slog.Error("static sub failed", "err", err)
	} else {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	}

	// API
	mux.HandleFunc("GET /api/list", makeHandler(store, cache, handleAPIList))
	mux.HandleFunc("GET /api/manga/{mhash}", makeHandler(store, cache, handleAPIManga))
	mux.HandleFunc("GET /api/similar/{mhash}", makeHandler(store, cache, handleAPISimilar))
	mux.HandleFunc("GET /api/search", makeHandler(store, cache, handleAPISearch))
	mux.HandleFunc("GET /thumb/{mhash}", makeHandler(store, cache, handleThumb))
	mux.HandleFunc("GET /g/{mhash}/img/{n}", makeHandler(store, cache, handleImage))

	// SPA catch-all
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(indexHTML)
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("server listening", "addr", addr)
	if err := http.ListenAndServe(addr, logged(mux)); err != nil {
		slog.Error("server error", "err", err)
	}
}

type handlerCtx struct {
	store *Store
	cache *zipCache
}

func makeHandler(store *Store, cache *zipCache, fn func(*handlerCtx, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	ctx := &handlerCtx{store: store, cache: cache}
	return func(w http.ResponseWriter, r *http.Request) {
		fn(ctx, w, r)
	}
}

func handleAPIList(ctx *handlerCtx, w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	sortBy := queryString(r, "sort", "mtime")
	if sortBy != "title" {
		sortBy = "mtime"
	}

	items, total, err := ctx.store.ListManga(page, 20, sortBy)
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
		"per_page": 20,
	})
}

func handleAPIManga(ctx *handlerCtx, w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	d, err := ctx.store.GetManga(mhash)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, d)
}

func handleAPISimilar(ctx *handlerCtx, w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	items, err := ctx.store.SimilarManga(mhash, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []MangaListItem{}
	}
	writeJSON(w, items)
}

func handleAPISearch(ctx *handlerCtx, w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := queryInt(r, "page", 1)
	sortBy := queryString(r, "sort", "mtime")
	if sortBy != "title" {
		sortBy = "mtime"
	}
	slog.Debug("search", "q", q, "fts", buildFTSQuery(q))

	items, total, err := ctx.store.Search(q, page, 20, sortBy)
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
		"per_page": 20,
	})
}

func handleThumb(ctx *handlerCtx, w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	data, err := ctx.store.GetThumbnail(mhash)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(data)
}

func handleImage(ctx *handlerCtx, w http.ResponseWriter, r *http.Request) {
	mhash := r.PathValue("mhash")
	nStr := r.PathValue("n")
	n, err := strconv.Atoi(nStr)
	if err != nil || n < 1 {
		http.Error(w, "invalid page", http.StatusBadRequest)
		return
	}

	path, err := ctx.store.GetFilePathForMhash(mhash)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx.cache.serveImage(w, mhash, path, n)
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
