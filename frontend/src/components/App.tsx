import { signal } from "@preact/signals";
export const lastSearchQuery = signal("");
import { Library } from "./Library";
import { Detail } from "./Detail";
import { Reader } from "./Reader";
import { Search } from "./Search";

history.scrollRestoration = "manual";

export const currentPath = signal(location.pathname + location.search);
export const previousPath = signal("");

function queryOf(path: string): string {
  return new URLSearchParams(path.split("?")[1] ?? "").get("q") ?? "";
}

// Persist the leaving page's state onto its history entry so back/forward can restore it.
function savePageState() {
  history.replaceState(
    { ...history.state, scrollY: window.scrollY, search: lastSearchQuery.value },
    "",
  );
}

// Restore the scroll position saved on the current history entry, after the page has rendered.
export function restoreScroll() {
  const y = history.state?.scrollY;
  if (y) requestAnimationFrame(() => window.scrollTo(0, y));
}

lastSearchQuery.value = history.state?.search ?? queryOf(currentPath.value);

export function navigate(path: string) {
  savePageState();
  previousPath.value = currentPath.value;
  if (path.startsWith("/search")) lastSearchQuery.value = queryOf(path);
  history.pushState({ search: lastSearchQuery.value }, "", path);
  currentPath.value = path;
}

window.addEventListener("popstate", () => {
  const path = location.pathname + location.search;
  lastSearchQuery.value = history.state?.search ?? queryOf(path);
  currentPath.value = path;
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
    const sort = params.get("sort") ?? "title";
    return <Search q={q} page={page} sort={sort} />;
  }

  // /  — library
  const page = parseInt(params.get("page") ?? "1", 10);
  const sort = params.get("sort") ?? "mtime";
  return <Library page={page} sort={sort} />;
}
