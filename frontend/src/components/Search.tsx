import { useState, useEffect } from "preact/hooks";
import { fetchSearch, ListResponse } from "../api";
import { navigate, lastSearchQuery } from "./App";
import { CardGrid, Pagination, goRandom } from "./Library";

interface Props {
  q: string;
  page: number;
  sort: string;
}

function SearchHeader({ q, sort }: { q: string; sort: string }) {
  const [input, setInput] = useState(q);

  useEffect(() => setInput(q), [q]);

  function submit(e: Event) {
    e.preventDefault();
    if (input.trim()) navigate(`/search?q=${encodeURIComponent(input.trim())}&sort=${sort}`);
  }

  return (
    <div class="header">
      <h1 onClick={() => navigate("/")}>Mangoo</h1>
      <form class="search-form" onSubmit={submit}>
        <input
          class="search-input"
          type="text"
          value={input}
          onInput={(e) => setInput((e.target as HTMLInputElement).value)}
          placeholder="Search…"
          autofocus
        />
        <button class="btn" type="submit">Search</button>
      </form>
      <button class="btn btn-blue" onClick={() => goRandom(q)}>Random</button>
      <div class="sort-toggle">
        <button
          class={`btn btn-secondary${sort === "mtime" ? " active" : ""}`}
          onClick={() => navigate(`/search?q=${encodeURIComponent(q)}&sort=mtime`)}
        >Newest</button>
        <button
          class={`btn btn-secondary${sort === "title" ? " active" : ""}`}
          onClick={() => navigate(`/search?q=${encodeURIComponent(q)}&sort=title`)}
        >A–Z</button>
      </div>
    </div>
  );
}

export function Search({ q, page, sort }: Props) {
  const [data, setData] = useState<ListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    lastSearchQuery.value = q;
  }, [q]);

  useEffect(() => {
    setData(null);
    setError(null);
    fetchSearch(q, page, sort).then(setData).catch((e) => setError(e.message));
  }, [q, page, sort]);

  function setPage(p: number) {
    navigate(`/search?q=${encodeURIComponent(q)}&page=${p}&sort=${sort}`);
  }

  return (
    <>
      <SearchHeader q={q} sort={sort} />
      <div class="page-wrap">
        {data && (
          <div class="search-heading">
            {data.total} found
          </div>
        )}
        {error && <div class="status">Error: {error}</div>}
        {!data && !error && <div class="status">Searching…</div>}
        {data && (
          <>
            {data.manga.length === 0 && <div class="status">No results.</div>}
            <CardGrid items={data.manga} />
            <Pagination page={page} total={data.total} perPage={data.per_page} onPage={setPage} />
          </>
        )}
      </div>
    </>
  );
}
