import { useState, useEffect, useLayoutEffect } from "preact/hooks";
import { fetchManga, fetchSimilar, MangaDetail, MangaListItem, Tag } from "../api";
import { navigate } from "./App";
import { Header, CardGrid, goRandom } from "./Library";

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
  useLayoutEffect(() => {
    window.scrollTo(0, 0);
  }, [mhash]);

  useEffect(() => {
    setManga(null);
    setError(null);
    setSimilar(null);
    fetchManga(mhash).then(setManga).catch((e) => setError(e.message));
    fetchSimilar(mhash).then(setSimilar).catch(() => setSimilar([]));
  }, [mhash]);

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
        <div class="detail-cover" onClick={() => manga && navigate(`/g/${mhash}/1`)}>
          <img src={`/thumb/${mhash}`} alt="" aria-hidden="true" />
          <img
            class="detail-cover-full"
            src={`/g/${mhash}/img/1?w=680`}
            alt={manga?.title ?? ""}
            style={{ opacity: 0 }}
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
      {similar && similar.length > 0 && (
        <div class="similar-section">
          <h2 class="similar-heading">Similar</h2>
          <CardGrid items={similar} />
        </div>
      )}
    </>
  );
}
