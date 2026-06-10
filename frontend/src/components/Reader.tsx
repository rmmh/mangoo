import { useState, useEffect, useRef } from "preact/hooks";
import { JSX } from "preact";
import { fetchManga } from "../api";
import { navigate } from "./App";

const resizeParams = (() => {
  const dpr = window.devicePixelRatio || 1;
  const w = Math.round(window.screen.width * dpr);
  const h = Math.round(window.screen.height * dpr);
  return `?w=${w}&h=${h}`;
})();

function TimedImg({ onSlow, ...props }: { onSlow: () => void } & JSX.HTMLAttributes<HTMLImageElement>) {
  const startRef = useRef(performance.now());
  return (
    <img
      {...props}
      onLoad={() => {
        if (performance.now() - startRef.current > 1000) onSlow();
      }}
    />
  );
}

interface Props {
  mhash: string;
  initialPage: number;
}

export function Reader({ mhash, initialPage }: Props) {
  const [page, setPage] = useState(initialPage);
  const [manga, setManga] = useState<{ title: string; page_count: number } | null>(null);
  const pageCount = manga?.page_count ?? null;
  const [picking, setPicking] = useState(false);
  const [pickInput, setPickInput] = useState("");
  const [useResize, setUseResize] = useState(window.screen.width <= 600);
  const inputRef = useRef<HTMLInputElement>(null);
  const imageWrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    fetchManga(mhash)
      .then((m) => setManga({ title: m.title, page_count: m.page_count }))
      .catch(() => setManga({ title: "", page_count: 0 }));
  }, [mhash]);

  useEffect(() => {
    imageWrapRef.current?.scrollIntoView({ behavior: "instant" as ScrollBehavior });
  }, [page]);

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

  const progress = pageCount && pageCount > 1 ? (page - 1) / (pageCount - 1) : 0;

  return (
    <div class="reader" style={{ '--progress': progress } as any}>
      <div class="reader-header">
        <button class="btn btn-secondary reader-back" onClick={() => navigate(`/g/${mhash}`)}>←</button>
        <span class="reader-title">{manga?.title ?? ""}</span>
        <span class="reader-page" onClick={openPicker}>
          {page}{pageCount ? ` / ${pageCount}` : ""}
        </span>
      </div>

      <div class="reader-image-wrap" ref={imageWrapRef}>
        <TimedImg
          key={page}
          src={`/g/${mhash}/img/${page}${useResize ? resizeParams : ""}`}
          alt={`Page ${page}`}
          onSlow={() => setUseResize(true)}
        />
        {page > 1 && <img key={page - 1} class="reader-prefetch" src={`/g/${mhash}/img/${page - 1}${useResize ? resizeParams : ""}`} aria-hidden="true" />}
        {pageCount !== null && page < pageCount && <img key={page + 1} class="reader-prefetch" src={`/g/${mhash}/img/${page + 1}${useResize ? resizeParams : ""}`} aria-hidden="true" />}
        <div class="reader-zone reader-zone-left" onClick={() => goPage(page - 1)} />
        <div class="reader-zone reader-zone-right" onClick={() => goPage(page + 1)} />
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
