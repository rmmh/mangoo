# Mangoo

Dead-simple manga reader. Go + TypeScript + Preact + esbuild + SQLite3 (modernc, pure Go).

## Build

```
make build        # frontend-build then go build -o mangoo
air               # dev: watches .go/.ts/.tsx/.css/.html, rebuilds+restarts
```

Requires `libwebp-dev` (`CGO_ENABLED=1`). esbuild outputs to `frontend/dist/app.js` (embedded via `//go:embed`).

## Running

Config search: `--config` flag → `./mangoo.toml` → `~/.config/mangoo/config.toml`.

```toml
port = 8080
db_path = "mangoo.db"
libraries = ["/path/to/manga"]
```

`--verbose` enables debug logging (per-request, per-file scan decisions, FTS queries).

## Architecture

```
main.go         entry: config, DB, start scanner+thumbnailer goroutines, runServer
config.go       TOML Config struct + loadConfig()
db.go           Store struct, schema init, all query methods, FTS helpers
hash.go         ComputeMHash: base32(sha256(size||first256k||last256k))[:10]
scanner.go      library walker, upsert, cascade delete, 4h periodic loop
thumbnailer.go  background goroutine: WebP resize+encode, batches of 16
zipcache.go     LRU-8 open zip.ReadClosers with sorted image slices
ziputil.go      filterAndSortImages shared by scanner, thumbnailer, zipcache
similar.go      SimilarManga: tag-weight + trigram-Jaccard scoring
server.go       HTTP mux, handlers, logging middleware
stream.go       GET /api/thumbs: streams length-prefixed WebP thumbnails for a gallery
embed.go        //go:embed for frontend/dist and mangoo.example.toml
frontend/src/   Preact SPA
  api.ts        typed fetch wrappers
  components/
    App.tsx     signal-based router (no library)
    Library.tsx card grid + pagination + search bar
    Detail.tsx  cover + metadata + tag chips
    Reader.tsx  full-viewport reader with tap zones
    Search.tsx  search results (same card grid)
```

## Database

SQLite with WAL + incremental vacuum. Tables: `file`, `manga`, `thumbnail`, `search` (FTS5 trigram).

MHASH: `base32(sha256(uint64LE(size) || first256KiB || last256KiB))[:16]` — 80 bits

FTS5 columns: `mhash` (UNINDEXED), `title`, `artist`, `category`, `character`, `group_col`, `language`, `parody`, `tag`, `tags` (all names concatenated). `group` tag type → `group_col` (avoids SQL reserved word).

## API Routes

```
GET  /api/list              ?page=N&sort=mtime|title
GET  /api/manga/{mhash}
GET  /api/similar/{mhash}
GET  /api/search            ?q=...&page=N&sort=...
GET  /api/random            ?q=...
POST /api/rescan
GET  /api/thumbs            ?m=mhash&o=offset&w=W&h=H (streaming WebP thumbnails)
GET  /thumb/{mhash}
GET  /g/{mhash}/img/{n}
GET  /static/               embedded frontend/dist
/                           SPA catch-all
```

## Frontend Routing

Signal-based, no router lib. `currentPath` signal + `navigate()` + `popstate` listener.

- `/` or `/?page=N` → Library
- `/g/:mhash` → Detail
- `/g/:mhash/:n` → Reader
- `/search?q=...` → Search

## Style

- No comments unless the WHY is non-obvious. Never describe what the code does.
- No abstractions beyond what the task requires.
- No error handling for impossible cases; trust internal guarantees.
- Go: `slog.Info`/`slog.Debug` (no logger threading through structs — use default logger).
- TypeScript: functional components, `useState`/`useEffect` from preact/hooks, `@preact/signals` for global state.
- CSS: hand-written in `frontend/src/app.css`, no framework.
