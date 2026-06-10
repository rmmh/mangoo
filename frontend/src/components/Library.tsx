import { useState, useEffect } from "preact/hooks";
import { fetchList, fetchRandom, fetchRescan, MangaListItem, ListResponse } from "../api";
import { navigate, lastSearchQuery, currentPath } from "./App";

interface Props {
  page: number;
  sort: string;
}

function SearchBar() {
  const signalQ = lastSearchQuery.value;
  const [input, setInput] = useState(signalQ);
  useEffect(() => { setInput(signalQ); }, [signalQ]);

  function submit(e: Event) {
    e.preventDefault();
    const path = currentPath.value;
    const isSearch = path.startsWith("/search");
    const sort = new URLSearchParams(path.split("?")[1] ?? "").get("sort") ?? "title";
    if (input.trim()) navigate(`/search?q=${encodeURIComponent(input.trim())}${isSearch ? `&sort=${sort}` : ""}`);
  }

  return (
    <form class="search-form" onSubmit={submit}>
      <input
        class="search-input"
        type="text"
        placeholder="Search titles, tags, artists…"
        value={input}
        onInput={(e) => setInput((e.target as HTMLInputElement).value)}
      />
      <button class="btn" type="submit">Search</button>
    </form>
  );
}

export function Header() {
  const path = currentPath.value;
  const isSearch = path.startsWith("/search");
  const params = new URLSearchParams(path.split("?")[1] ?? "");
  const sort = params.get("sort") ?? (isSearch ? "title" : "mtime");
  const q = params.get("q") ?? "";

  function setSort(s: string) {
    if (isSearch) navigate(`/search?q=${encodeURIComponent(q)}&sort=${s}`);
    else navigate(`/?sort=${s}`);
  }

  return (
    <div class="header">
      <h1 onClick={() => navigate("/")}>Mangoo</h1>
      <SearchBar />
      <div class="header-actions">
        <button class="btn btn-blue" onClick={() => goRandom(lastSearchQuery.value)}>Random</button>
        <div class="sort-toggle">
          <button class={`btn btn-secondary${sort === "mtime" ? " active" : ""}`} onClick={() => setSort("mtime")}>Newest</button>
          <button class={`btn btn-secondary${sort === "title" ? " active" : ""}`} onClick={() => setSort("title")}>A–Z</button>
        </div>
      </div>
    </div>
  );
}

export function goRandom(q: string) {
  fetchRandom(q).then((r) => navigate(`/g/${r.mhash}`)).catch(() => { });
}

export function CardGrid({ items }: { items: MangaListItem[] }) {
  return (
    <div class="card-grid">
      {items.map((m) => (
        <div class="card" key={m.mhash} onClick={() => navigate(`/g/${m.mhash}`)}>
          <img class="card-thumb" src={`/thumb/${m.mhash}`} alt={m.title} loading="lazy" />
          <div class="card-title">{m.title}</div>
        </div>
      ))}
    </div>
  );
}

export function Pagination({ page, total, perPage, onPage }: { page: number; total: number; perPage: number; onPage: (p: number) => void }) {
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  return (
    <div class="pagination">
      <button class={`btn btn-secondary${page <= 1 ? " invisible" : ""}`} onClick={() => onPage(page - 1)}>← Prev</button>
      <div class="pagination-info">
        {totalPages > 1 && <div>Page {page} / {totalPages}</div>}
        <div>{total.toLocaleString()} items</div>
      </div>
      <button class={`btn btn-secondary${page >= totalPages ? " invisible" : ""}`} onClick={() => onPage(page + 1)}>Next →</button>
    </div>
  );
}

function RescanButton() {
  const [state, setState] = useState<"idle" | "scanning" | "done">("idle");

  function rescan() {
    setState("scanning");
    fetchRescan()
      .then(() => setState("done"))
      .catch(() => setState("idle"));
  }

  return (
    <button
      class="btn btn-secondary"
      onClick={rescan}
      disabled={state === "scanning"}
    >
      {state === "scanning" ? "Scanning…" : state === "done" ? "Scan queued" : "Rescan"}
    </button>
  );
}

export function RescanRow({ files_scanned, thumb_backlog }: { files_scanned: number; thumb_backlog: number }) {
  return (
    <div class="rescan-row">
      <RescanButton />
      {(files_scanned > 0 || thumb_backlog > 0) && (
        <div class="scan-stats">
          {files_scanned > 0 && `Scanned ${files_scanned.toLocaleString()} files`}
          {files_scanned > 0 && thumb_backlog > 0 && ", "}
          {thumb_backlog > 0 && `thumbnailer backlog is ${thumb_backlog.toLocaleString()} files`}
        </div>
      )}
    </div>
  );
}

export function Library({ page, sort }: Props) {
  const [data, setData] = useState<ListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setError(null);
    fetchList(page, sort).then((d) => {
      setData(d);
      const y = history.state?.scrollY;
      if (y) requestAnimationFrame(() => window.scrollTo(0, y));
    }).catch((e) => setError(e.message));
  }, [page, sort]);

  function setPage(p: number) {
    navigate(`/?page=${p}&sort=${sort}`);
  }

  return (
    <>
      <Header />
      <div class="page-wrap">
        {error && <div class="status">Error: {error}</div>}
        {!data && !error && <div class="status">Loading…</div>}
        {data && (
          <>
            <Pagination page={page} total={data.total} perPage={data.per_page} onPage={setPage} />
            <CardGrid items={data.manga} />
            <Pagination page={page} total={data.total} perPage={data.per_page} onPage={setPage} />
            <RescanRow files_scanned={data.files_scanned} thumb_backlog={data.thumb_backlog} />
          </>
        )}
      </div>
    </>
  );
}
