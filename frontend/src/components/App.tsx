import { signal } from "@preact/signals";
import { Library } from "./Library";
import { Detail } from "./Detail";
import { Reader } from "./Reader";
import { Search } from "./Search";

export const currentPath = signal(location.pathname + location.search);

export function navigate(path: string) {
  history.pushState(null, "", path);
  currentPath.value = path;
}

window.addEventListener("popstate", () => {
  currentPath.value = location.pathname + location.search;
});

export function App() {
  const path = currentPath.value;
  const [pathname, search] = path.split("?");
  const params = new URLSearchParams(search ?? "");

  // /g/:mhash/:n  — reader
  const readerMatch = pathname.match(/^\/g\/([^/]+)\/(\d+)$/);
  if (readerMatch) {
    return <Reader mhash={readerMatch[1]} initialPage={parseInt(readerMatch[2], 10)} />;
  }

  // /g/:mhash  — detail
  const detailMatch = pathname.match(/^\/g\/([^/]+)$/);
  if (detailMatch) {
    return <Detail mhash={detailMatch[1]} />;
  }

  // /search
  if (pathname === "/search") {
    const q = params.get("q") ?? "";
    const page = parseInt(params.get("page") ?? "1", 10);
    const sort = params.get("sort") ?? "mtime";
    return <Search q={q} page={page} sort={sort} />;
  }

  // /  — library
  const page = parseInt(params.get("page") ?? "1", 10);
  const sort = params.get("sort") ?? "mtime";
  return <Library page={page} sort={sort} />;
}
