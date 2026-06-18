import { useState, useEffect, useLayoutEffect, useRef } from "preact/hooks";
import { fetchManga, fetchSimilar, streamThumbs, MangaDetail, MangaListItem, Tag } from "../api";
import { navigate, previousPath, restoreScroll } from "./App";
import { Header, CardGrid, goRandom, VersionFooter } from "./Library";

const THUMB_CACHE_MAX = 200 * 1024 * 1024;
const thumbCache = new Map<string, Uint8Array>();
let thumbCacheBytes = 0;

function cacheSet(key: string, data: Uint8Array) {
  if (thumbCache.has(key)) return;
  thumbCache.set(key, data);
  thumbCacheBytes += data.byteLength;
  while (thumbCacheBytes > THUMB_CACHE_MAX) {
    const [k, v] = thumbCache.entries().next().value!;
    thumbCache.delete(k);
    thumbCacheBytes -= v.byteLength;
  }
}

function cacheGet(key: string): Uint8Array | undefined {
  const v = thumbCache.get(key);
  if (!v) return undefined;
  thumbCache.delete(key);
  thumbCache.set(key, v);
  return v;
}

interface Props {
  mhash: string;
}

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / (1024 * 1024)).toFixed(1)} MB`;
}

function TagGroup({ kind, tags }: { kind: string; tags: Tag[] }) {
  return (
    <div class="tag-group">
      <div class="tag-kind">{kind}</div>
      <div class="tag-chips">
        {tags.map((t) => (
          <span
            class="tag-chip"
            key={`${t.type}:${t.name}`}
            onClick={() => navigate(`/search?q=${encodeURIComponent(`${t.type}:"${t.name}"`)}`)}
          >
            {t.name}
          </span>
        ))}
      </div>
    </div>
  );
}

const TAG_ORDER = ["artist", "parody", "character", "group", "category", "language", "tag"];

export function Detail({ mhash }: Props) {
  const [manga, setManga] = useState<MangaDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [similar, setSimilar] = useState<MangaListItem[] | null>(null);
  const [pageThumbs, setPageThumbs] = useState<Map<number, string>>(new Map());
  const [totalPages, setTotalPages] = useState(0);
  const stripRef = useRef<HTMLDivElement>(null);
  const [readerBack] = useState<string | null>(() => {
    const prev = previousPath.value;
    return /^\/g\/[^/]+\/\d+$/.test(prev) ? prev : null;
  });
  useLayoutEffect(() => {
    if (!history.state?.scrollY) window.scrollTo(0, 0);
    const revoke = (prev: Map<number, string>) => {
      prev.forEach(URL.revokeObjectURL);
      return new Map<number, string>();
    };
    setTotalPages(0);
    setPageThumbs(revoke);
    return () => setPageThumbs(revoke);
  }, [mhash]);

  useEffect(() => {
    setManga(null);
    setError(null);
    setSimilar(null);
    fetchManga(mhash).then(setManga).catch((e) => setError(e.message));
    fetchSimilar(mhash).then(setSimilar).catch(() => setSimilar([]));
  }, [mhash]);

  useEffect(() => {
    const initial = new Map<number, string>();
    let startPage = 1;
    for (; ;) {
      const cached = cacheGet(`${mhash}:${startPage}`);
      if (!cached) break;
      initial.set(startPage, URL.createObjectURL(new Blob([cached], { type: "image/webp" })));
      startPage++;
    }
    setPageThumbs(initial);
    setTotalPages(startPage - 1); // known from cache; updated when stream header arrives

    const ac = new AbortController();
    const offset = startPage - 1;
    const el = stripRef.current?.firstElementChild as HTMLElement | null;
    const thumbW = el?.offsetWidth ?? 100;
    const thumbH = el?.offsetHeight ?? Math.floor(thumbW * 4 / 3);

    streamThumbs(mhash, offset, thumbW, thumbH, ac.signal, (total) => setTotalPages(total), (page, data) => {
      cacheSet(`${mhash}:${page}`, data);
      const url = URL.createObjectURL(new Blob([data], { type: "image/webp" }));
      setPageThumbs((prev) => new Map(prev).set(page, url));
    });

    return () => ac.abort();
  }, [mhash]);

  useEffect(() => {
    restoreScroll();
  }, [mhash, totalPages, similar]);

  useEffect(() => {
    if (!readerBack) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "ArrowLeft") { e.preventDefault(); navigate(readerBack!); }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [readerBack]);

  const grouped = new Map<string, Tag[]>();
  if (manga?.tags) {
    for (const t of manga.tags) {
      const list = grouped.get(t.type) ?? [];
      list.push(t);
      grouped.set(t.type, list);
    }
  }

  const orderedKinds = [
    ...TAG_ORDER.filter((k) => grouped.has(k)),
    ...[...grouped.keys()].filter((k) => !TAG_ORDER.includes(k)),
  ];

  return (
    <>
      <Header />
      <div class="detail-wrap">
        {error && <div class="status">Error: {error}</div>}
        {readerBack && (
          <button class="btn btn-secondary reader-back" onClick={() => navigate(readerBack)}>←</button>
        )}
        <div class="detail-cover" onClick={() => manga && navigate(`/g/${mhash}/1`)}>
          <img key={mhash} src={`/thumb/${mhash}`} alt="" aria-hidden="true" />
          <img
            key={mhash}
            class="detail-cover-full"
            src={`/g/${mhash}/img/1?w=680`}
            alt={manga?.title ?? ""}
            onLoad={(e) => { (e.currentTarget as HTMLImageElement).style.opacity = "1"; }}
          />
        </div>
        {!manga && !error && <div class="status">Loading…</div>}
        {manga && (
          <>
            <div class="detail-info">
              <h1 class="detail-title">{manga.title}</h1>
              <div class="detail-meta">
                <div><strong>Pages:</strong> {manga.page_count}</div>
                <div><strong>Size:</strong> {formatBytes(manga.file_size)}</div>
                <div><strong>Path:</strong> {manga.file_path}</div>
              </div>
              {orderedKinds.length > 0 && (
                <div class="tags-section">
                  {orderedKinds.map((k) => (
                    <TagGroup key={k} kind={k} tags={grouped.get(k)!} />
                  ))}
                </div>
              )}
            </div>
          </>
        )}
      </div>
      <div ref={stripRef} class="page-strip">
        {Array.from({ length: Math.max(totalPages, 1) }, (_, i) => i + 1).map((page) => {
          const url = pageThumbs.get(page);
          return (
            <div key={page} class="page-thumb" onClick={() => navigate(`/g/${mhash}/${page}`)}>
              {url && <img src={url} alt={`Page ${page}`} />}
            </div>
          );
        })}
      </div>
      {similar && similar.length > 0 && (
        <div class="similar-section">
          <h2 class="similar-heading">Similar</h2>
          <CardGrid items={similar} />
        </div>
      )}
      <VersionFooter />
    </>
  );
}
