export interface MangaListItem {
  mhash: string;
  title: string;
  mtime: number;
  page_count: number;
}

export interface Tag {
  type: string;
  name: string;
}

export interface MangaDetail {
  mhash: string;
  title: string;
  mtime: number;
  page_count: number;
  file_path: string;
  file_size: number;
  tags: Tag[];
}

export interface ListResponse {
  manga: MangaListItem[];
  total: number;
  page: number;
  per_page: number;
  files_scanned: number;
  thumb_backlog: number;
}

export async function fetchList(page: number, sort: string): Promise<ListResponse> {
  const r = await fetch(`/api/list?page=${page}&sort=${sort}`);
  if (!r.ok) throw new Error(`list: ${r.status}`);
  return r.json();
}

export async function fetchManga(mhash: string): Promise<MangaDetail> {
  const r = await fetch(`/api/manga/${mhash}`);
  if (!r.ok) throw new Error(`manga: ${r.status}`);
  return r.json();
}

export async function fetchSearch(q: string, page: number, sort: string): Promise<ListResponse> {
  const r = await fetch(`/api/search?q=${encodeURIComponent(q)}&page=${page}&sort=${sort}`);
  if (!r.ok) throw new Error(`search: ${r.status}`);
  return r.json();
}

export async function fetchRescan(): Promise<void> {
  const r = await fetch("/api/rescan", { method: "POST" });
  if (!r.ok) throw new Error(`rescan: ${r.status}`);
}

export async function fetchRandom(q: string): Promise<{ mhash: string }> {
  const r = await fetch(`/api/random?q=${encodeURIComponent(q)}`);
  if (!r.ok) throw new Error(`random: ${r.status}`);
  return r.json();
}

export async function fetchSimilar(mhash: string): Promise<MangaListItem[]> {
  const r = await fetch(`/api/similar/${mhash}`);
  if (!r.ok) throw new Error(`similar: ${r.status}`);
  return r.json();
}

export async function streamThumbs(
  mhash: string,
  offset: number,
  w: number,
  h: number,
  signal: AbortSignal,
  onCount: (total: number) => void,
  onThumb: (page: number, data: Uint8Array) => void,
): Promise<void> {
  let resp: Response;
  try {
    resp = await fetch(`/api/thumbs?m=${mhash}&o=${offset}&w=${w}&h=${h}`, { signal });
  } catch (e: any) {
    if (e?.name !== "AbortError") throw e;
    return;
  }
  if (!resp.ok || !resp.body) return;

  const reader = resp.body.getReader();
  let buf = new Uint8Array(0);
  let page = offset + 1; // 1-based page number
  let countRead = false;

  for (;;) {
    let done: boolean, value: Uint8Array | undefined;
    try {
      ({ done, value } = await reader.read());
    } catch {
      return; // aborted
    }
    if (done) break;
    if (!value) continue;

    const next = new Uint8Array(buf.length + value.length);
    next.set(buf);
    next.set(value, buf.length);
    buf = next;

    if (!countRead && buf.length >= 4) {
      const remaining = new DataView(buf.buffer, buf.byteOffset, 4).getUint32(0);
      onCount(offset + remaining);
      buf = buf.slice(4);
      countRead = true;
    }

    // Parse complete length-prefixed chunks: 4-byte big-endian size + data
    while (countRead && buf.length >= 4) {
      const size = new DataView(buf.buffer, buf.byteOffset, 4).getUint32(0);
      if (buf.length < 4 + size) break;
      onThumb(page++, buf.slice(4, 4 + size));
      buf = buf.slice(4 + size);
    }
  }
}
