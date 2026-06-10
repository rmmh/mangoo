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

export async function fetchSimilar(mhash: string): Promise<MangaListItem[]> {
  const r = await fetch(`/api/similar/${mhash}`);
  if (!r.ok) throw new Error(`similar: ${r.status}`);
  return r.json();
}
