import { useState, useEffect, useRef } from "preact/hooks";
import { fetchManga } from "../api";
import { navigate } from "./App";

interface Props {
  mhash: string;
  initialPage: number;
}

export function Reader({ mhash, initialPage }: Props) {
  const [page, setPage] = useState(initialPage);
  const [pageCount, setPageCount] = useState<number | null>(null);
  const [picking, setPicking] = useState(false);
  const [pickInput, setPickInput] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    fetchManga(mhash).then((m) => setPageCount(m.page_count)).catch(() => setPageCount(0));
  }, [mhash]);

  function goPage(n: number) {
    if (n < 1) return;
    if (pageCount !== null && n > pageCount) { navigate(`/g/${mhash}`); return; }
    setPage(n);
    history.replaceState(null, "", `/g/${mhash}/${n}`);
  }

  function openPicker() {
    setPickInput(String(page));
    setPicking(true);
    setTimeout(() => inputRef.current?.select(), 0);
  }

  function commitPick() {
    const n = parseInt(pickInput, 10);
    if (!isNaN(n)) goPage(n);
    setPicking(false);
  }

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (picking) {
        if (e.key === "Escape") setPicking(false);
        return;
      }
      if (e.key === "ArrowRight" || e.key === " ") { e.preventDefault(); goPage(page + 1); }
      if (e.key === "ArrowLeft") { e.preventDefault(); goPage(page - 1); }
      if (e.key === "Escape") navigate(`/g/${mhash}`);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [page, pageCount, picking]);

  return (
    <div class="reader">
      <img key={page} src={`/g/${mhash}/img/${page}`} alt={`Page ${page}`} />
      {page > 1 && <img key={page - 1} class="reader-prefetch" src={`/g/${mhash}/img/${page - 1}`} aria-hidden="true" />}
      {pageCount !== null && page < pageCount && <img key={page + 1} class="reader-prefetch" src={`/g/${mhash}/img/${page + 1}`} aria-hidden="true" />}

      <div class="reader-zone reader-zone-left" onClick={() => goPage(page - 1)} />
      <div class="reader-zone reader-zone-right" onClick={() => goPage(page + 1)} />

      <div class="reader-bar">
        <span class="reader-page" onClick={openPicker}>
          {page}{pageCount ? ` / ${pageCount}` : ""}
        </span>
        <button class="btn btn-secondary" onClick={() => navigate(`/g/${mhash}`)}>?</button>
      </div>

      {picking && (
        <div class="page-picker-backdrop" onClick={() => setPicking(false)}>
          <form class="page-picker" onClick={(e) => e.stopPropagation()} onSubmit={(e) => { e.preventDefault(); commitPick(); }}>
            <input
              ref={inputRef}
              class="page-picker-input"
              type="number"
              min={1}
              max={pageCount ?? undefined}
              value={pickInput}
              onInput={(e) => {
                const val = (e.target as HTMLInputElement).value;
                setPickInput(val);
                const n = parseInt(val, 10);
                if (!isNaN(n)) goPage(n);
              }}
            />
          </form>
        </div>
      )}
    </div>
  );
}
