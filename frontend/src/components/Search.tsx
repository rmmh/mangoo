import { useState, useEffect } from "preact/hooks";
import { fetchSearch, ListResponse } from "../api";
import { navigate, restoreScroll } from "./App";
import { Header, CardGrid, Pagination, RescanRow, VersionFooter } from "./Library";

interface Props {
  q: string;
  page: number;
  sort: string;
}

export function Search({ q, page, sort }: Props) {
  const [data, setData] = useState<ListResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setError(null);
    fetchSearch(q, page, sort).then((d) => {
      setData(d);
      restoreScroll();
    }).catch((e) => setError(e.message));
  }, [q, page, sort]);

  function setPage(p: number) {
    navigate(`/search?q=${encodeURIComponent(q)}&page=${p}&sort=${sort}`);
  }

  return (
    <>
      <Header />
      <div class="page-wrap">
        {error && <div class="status">Error: {error}</div>}
        {!data && !error && <div class="status">Searching…</div>}
        {data && (
          <>
            <Pagination page={page} total={data.total} perPage={data.per_page} onPage={setPage} />
            {data.manga.length === 0 && <div class="status">No results.</div>}
            <CardGrid items={data.manga} />
            <Pagination page={page} total={data.total} perPage={data.per_page} onPage={setPage} />
            <RescanRow files_scanned={data.files_scanned} thumb_backlog={data.thumb_backlog} />
          </>
        )}
      </div>
      <VersionFooter />
    </>
  );
}
