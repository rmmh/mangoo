# Mangoo

Dead-simple, self-hosted manga/comic reader. Point it at folders of archives and browse them in your browser.

> Why "Mangoo"? It's a manga reader in Go — but [Mango](https://github.com/getmango/Mango) (the Crystal one) already took the obvious name, and it's deprecated/unmaintained as of March 2025. So: one more `o`.

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

## Install

Grab a prebuilt Linux amd64 binary from the [latest release](https://github.com/rmmh/mangoo/releases/latest) (libwebp is statically linked, so no system libs needed):

```sh
curl -sfL https://github.com/rmmh/mangoo/releases/latest/download/mangoo-VERSION-linux-amd64.tar.gz | tar xz
./mangoo
```

Replace `VERSION` with the release tag (e.g. `v0.1.0`), or download from the releases page.

### Build from source

```sh
make build      # builds frontend then the mangoo binary
./mangoo
```

Requires `libwebp-dev` and Node (for the frontend build); `CGO_ENABLED=1`.

## Configuration

Config search order: `--config` flag → `./mangoo.toml` → `~/.config/mangoo/config.toml`.

```toml
port = 8080
db_path = "mangoo.db"
libraries = ["/path/to/manga"]
```

For development, `air` watches `.go/.ts/.tsx/.css/.html` and rebuilds + restarts.

See [CLAUDE.md](CLAUDE.md) for architecture and internals.

## License

[PolyForm Noncommercial 1.0.0](LICENSE) — free to use, modify, and share for any noncommercial purpose. Commercial use requires a separate license; ask. This is source-available, not OSI open source.

---

*Heads up: large parts of this codebase (and this README) are AI slop — generated with Claude. Read before you trust.*
