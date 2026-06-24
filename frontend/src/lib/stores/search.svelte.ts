import { SearchService } from "../api/generated/index";
import {
  configureGeneratedClient,
  isAbortError,
  withAbort,
} from "../api/runtime.js";
import { debounce } from "../utils/debounce.js";
import { fetchSemanticSearchStatus, semanticSearch } from "../api/llm.js";
import type {
  SearchResponse,
  SearchResult,
} from "../api/types.js";

class SearchStore {
  query: string = $state("");
  project: string = $state("");
  sort: "relevance" | "recency" = $state("relevance");
  mode: "keyword" | "semantic" = $state("keyword");
  semanticAvailable: boolean = $state(false);
  results: SearchResult[] = $state([]);
  isSearching: boolean = $state(false);

  private abortController: AbortController | null = null;
  private statusController: AbortController | null = null;

  private debouncedSearch = debounce(
    (q: string, project: string) => {
      this.executeSearch(q, project);
    },
    300,
  );

  search(q: string, project?: string) {
    this.query = q;
    if (project !== undefined) this.project = project;

    if (!q.trim()) {
      this.debouncedSearch.cancel();
      this.abortController?.abort();
      this.results = [];
      this.isSearching = false;
      return;
    }

    this.abortController?.abort();
    this.abortController = null;
    this.debouncedSearch(q, this.project);
  }

  setSort(s: "relevance" | "recency") {
    this.sort = s;
    if (this.query.trim()) {
      this.debouncedSearch.cancel();
      this.executeSearch(this.query, this.project);
    }
  }

  setMode(mode: "keyword" | "semantic") {
    if (mode === "semantic" && !this.semanticAvailable) return;
    this.mode = mode;
    if (this.query.trim()) {
      this.debouncedSearch.cancel();
      this.executeSearch(this.query, this.project);
    }
  }

  setSemanticAvailable(available: boolean) {
    this.semanticAvailable = available;
    if (!available && this.mode === "semantic") {
      this.mode = "keyword";
    }
  }

  async refreshSemanticAvailability() {
    this.statusController?.abort();
    this.statusController = new AbortController();
    try {
      const status = await fetchSemanticSearchStatus(this.statusController.signal);
      this.setSemanticAvailable(status.available === true);
    } catch {
      this.setSemanticAvailable(false);
    }
  }

  clear() {
    this.query = "";
    this.results = [];
    this.isSearching = false;
    this.debouncedSearch.cancel();
    this.abortController?.abort();
    this.statusController?.abort();
  }

  /** Full reset: clears results and resets sort to the default.
   * Call this on palette close, not on transient clears (e.g. query < 3 chars). */
  resetSort() {
    this.sort = "relevance";
    this.mode = "keyword";
  }

  private async executeSearch(
    q: string, project: string,
  ) {
    this.abortController?.abort();
    this.abortController = new AbortController();
    const { signal } = this.abortController;

    this.isSearching = true;
    try {
      configureGeneratedClient();
      const res = this.mode === "semantic"
        ? await semanticSearch(q, project || undefined, 30, signal)
        : await withAbort(
          SearchService.getApiV1Search({
            q,
            project: project || undefined,
            limit: 30,
            sort: this.sort,
          }) as unknown as Promise<SearchResponse>,
          signal,
        );
      if ("disabled" in res && res.disabled) {
        this.setSemanticAvailable(false);
        this.results = [];
        return;
      }
      if (this.mode === "semantic") {
        this.semanticAvailable = true;
      }
      this.results = res.results ?? [];
    } catch (error: unknown) {
      if (isAbortError(error)) {
        return;
      }
      this.results = [];
    } finally {
      if (!signal.aborted) {
        this.isSearching = false;
      }
    }
  }
}

export const searchStore = new SearchStore();
