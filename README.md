# Mangoo

Dead-simple, self-hosted manga/comic reader. Point it at folders of archives and browse them in your browser.

> Why "Mangoo"? It's a manga reader in Go — but [Mango](https://github.com/getmango/Mango) (the Crystal one) already took the obvious name, and it's [deprecated/unmaintained as of March 2025](https://github.com/getmango/Mango). So: one more `o`.

## Features

- **Archive support** — ZIP/CBZ, RAR/CBR (including solid), 7-Zip (including solid), and TAR, all read in place.
- **Library scanning** — walks configured folders, picks up changes, periodic rescan.
- **Full-text search** — SQLite FTS5 trigram over title, artist, tags, characters, group, language, parody.
- **Tag-aware "similar" suggestions** — tag-weight + trigram-Jaccard scoring.
- **Fast thumbnails** — background WebP encoding, streamed to the gallery grid.
- **Minimal reader** — full-viewport pages with tap zones, no chrome.
- **Single binary** — Go backend with the Preact frontend embedded via `go:embed`.

## Stack

Go · TypeScript · Preact · esbuild · SQLite (modernc, pure Go) · libwebp.

## Quick start

```sh
make build      # builds frontend then the mangoo binary
./mangoo
```

Requires `libwebp-dev` (`CGO_ENABLED=1`).

Config search order: `--config` flag → `./mangoo.toml` → `~/.config/mangoo/config.toml`.

```toml
port = 8080
db_path = "mangoo.db"
libraries = ["/path/to/manga"]
```

For development, `air` watches `.go/.ts/.tsx/.css/.html` and rebuilds + restarts.

See [CLAUDE.md](CLAUDE.md) for architecture and internals.

---

*Heads up: large parts of this codebase (and this README) are AI slop — generated with Claude. Read before you trust.*
