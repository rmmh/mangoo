import { useState, useEffect } from "preact/hooks";
import { fetchList, MangaListItem, ListResponse } from "../api";
import { navigate } from "./App";

interface Props {
  page: number;
  sort: string;
}

function SearchBar({ initialQ }: { initialQ?: string }) {
  const [q, setQ] = useState(initialQ ?? "");

  function submit(e: Event) {
    e.preventDefault();
    if (q.trim()) navigate(`/search?q=${encodeURIComponent(q.trim())}`);
  }

  return (
    <form class="search-form" onSubmit={submit}>
      <input
        class="search-input"
        type="text"
        placeholder="Search titles, tags, artists…"
        value={q}
        onInput={(e) => setQ((e.target as HTMLInputElement).value)}
      />
      <button class="btn" type="submit">Search</button>
    </form>
  );
}

export function Header({ sort, onSort }: { sort: string; onSort: (s: string) => void }) {
  return (
    <div class="header">
      <h1 onClick={() => navigate("/")}>Mangoo</h1>
      <SearchBar />
      <div class="sort-toggle">
        <button class={`btn btn-secondary${sort === "mtime" ? " active" : ""}`} onClick={() => onSort("mtime")}>Newest</button>
        <button class={`btn btn-secondary${sort === "title" ? " active" : ""}`} onClick={() => onSort("title")}>A–Z</button>
      </div>
    </div>
  );
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
  if (totalPages <= 1) return null;
  return (
    <div class="pagination">
      <button class="btn btn-secondary" disabled={page <= 1} onClick={() => onPage(page - 1)}>← Prev</button>
      <span>Page {page} / {totalPages}</span>
      <button class="btn btn-secondary" disabled={page >= totalPages} onClick={() => onPage(page + 1)}>Next →</button>
    </div>
  );
}

export function Library({ page, sort }: Props) {
  const [data, setData] = useState<ListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setData(null);
    setError(null);
    fetchList(page, sort).then(setData).catch((e) => setError(e.message));
  }, [page, sort]);

  function setSort(s: string) {
    navigate(`/?sort=${s}`);
  }

  function setPage(p: number) {
    navigate(`/?page=${p}&sort=${sort}`);
  }

  return (
    <>
      <Header sort={sort} onSort={setSort} />
      <div class="page-wrap">
        {error && <div class="status">Error: {error}</div>}
        {!data && !error && <div class="status">Loading…</div>}
        {data && (
          <>
            <CardGrid items={data.manga} />
            <Pagination page={page} total={data.total} perPage={data.per_page} onPage={setPage} />
          </>
        )}
      </div>
    </>
  );
}
