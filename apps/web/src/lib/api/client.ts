import type {
  Repo,
  Workspace,
  Change,
  Commit,
  Attestation,
  Suggestion,
  FileNode,
  FileContent,
  FileHistoryEntry,
  JulEvent,
  PaginatedResponse,
  PromoteResponse,
  ChangesQuery,
  AttestationsQuery,
  QueryParams,
  ApiError,
} from "./types";
import {
  mapArray,
  mapAttestation,
  mapChange,
  mapCommit,
  mapFileHistoryEntry,
  mapJobResponse,
  mapJulEvent,
  mapPromoteResponse,
  mapRepo,
  mapSuggestion,
  mapWorkspace,
  normalizePaginated,
} from "./mappers";

// Configuration
const DEFAULT_BASE_URL = process.env.NEXT_PUBLIC_JUL_API_URL || "http://localhost:8000";

export interface JulClientConfig {
  baseUrl?: string;
  token?: string;
}

type ResponseType = "auto" | "json" | "text";

type JulRequestOptions = RequestInit & {
  responseType?: ResponseType;
};

export class JulApiError extends Error {
  constructor(
    public status: number,
    public error: ApiError
  ) {
    super(error.message);
    this.name = "JulApiError";
  }
}

export class JulClient {
  private baseUrl: string;
  private token?: string;

  constructor(config: JulClientConfig = {}) {
    this.baseUrl = config.baseUrl || DEFAULT_BASE_URL;
    this.token = config.token;
  }

  setToken(token: string) {
    this.token = token;
  }

  private async request<T>(
    path: string,
    options: JulRequestOptions = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const { responseType = "auto", ...fetchOptions } = options;
    const headers: HeadersInit = {
      "Content-Type": "application/json",
      ...fetchOptions.headers,
    };

    if (this.token) {
      (headers as Record<string, string>)["Authorization"] = `Bearer ${this.token}`;
    }

    const response = await fetch(url, {
      ...fetchOptions,
      headers,
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({
        error: "unknown",
        message: response.statusText,
      }));
      throw new JulApiError(response.status, error);
    }

    // Handle empty responses
    const text = await response.text();
    if (!text) {
      return (responseType === "text" ? "" : {}) as T;
    }

    if (responseType === "text") {
      return text as T;
    }

    if (responseType === "json") {
      return JSON.parse(text) as T;
    }

    const contentType = response.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      return JSON.parse(text) as T;
    }

    return text as T;
  }

  // === Repositories ===

  async listRepos(): Promise<Repo[]> {
    const data = await this.request<unknown>("/api/v1/repos");
    return mapArray(data, mapRepo);
  }

  async getRepo(name: string): Promise<Repo> {
    const data = await this.request<unknown>(`/api/v1/repos/${name}`);
    return mapRepo(data);
  }

  async createRepo(data: {
    name: string;
    description?: string;
    visibility?: "public" | "private";
  }): Promise<Repo> {
    const response = await this.request<unknown>("/api/v1/repos", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return mapRepo(response);
  }

  async deleteRepo(name: string): Promise<void> {
    await this.request(`/api/v1/repos/${name}`, {
      method: "DELETE",
    });
  }

  // === Workspaces ===

  async listWorkspaces(repoName: string): Promise<Workspace[]> {
    const data = await this.request<unknown>(`/${repoName}.jul/api/v1/workspaces`);
    return mapArray(data, mapWorkspace);
  }

  async getWorkspace(repoName: string, workspaceId: string): Promise<Workspace> {
    const data = await this.request<unknown>(
      `/${repoName}.jul/api/v1/workspaces/${workspaceId}`
    );
    return mapWorkspace(data);
  }

  async promote(
    repoName: string,
    workspaceId: string,
    data: { targetBranch: string; commitSha?: string }
  ): Promise<PromoteResponse> {
    const response = await this.request<unknown>(
      `/${repoName}.jul/api/v1/workspaces/${workspaceId}/promote`,
      {
        method: "POST",
        body: JSON.stringify({
          target_branch: data.targetBranch,
          commit_sha: data.commitSha,
        }),
      }
    );
    return mapPromoteResponse(response);
  }

  // === Changes ===

  async listChanges(
    repoName: string,
    query?: ChangesQuery
  ): Promise<PaginatedResponse<Change>> {
    const params = new URLSearchParams();
    if (query?.status) params.set("status", query.status);
    if (query?.author) params.set("author", query.author);
    if (query?.limit) params.set("limit", String(query.limit));
    if (query?.offset) params.set("offset", String(query.offset));

    const queryString = params.toString();
    const path = `/${repoName}.jul/api/v1/changes${queryString ? `?${queryString}` : ""}`;
    const data = await this.request<unknown>(path);
    return normalizePaginated(data, mapChange);
  }

  async getChange(repoName: string, changeId: string): Promise<Change> {
    const data = await this.request<unknown>(
      `/${repoName}.jul/api/v1/changes/${changeId}`
    );
    return mapChange(data);
  }

  async getInterdiff(
    repoName: string,
    changeId: string,
    fromRev: number,
    toRev: number
  ): Promise<string> {
    return this.request<string>(
      `/${repoName}.jul/api/v1/changes/${changeId}/interdiff?from_rev=${fromRev}&to_rev=${toRev}`,
      {
        responseType: "text",
      }
    );
  }

  // === Commits & Attestations ===

  async getCommit(repoName: string, sha: string): Promise<Commit> {
    const data = await this.request<unknown>(`/${repoName}.jul/api/v1/commits/${sha}`);
    return mapCommit(data);
  }

  async getAttestation(repoName: string, sha: string): Promise<Attestation> {
    const data = await this.request<unknown>(
      `/${repoName}.jul/api/v1/commits/${sha}/attestation`
    );
    return mapAttestation(data);
  }

  async listAttestations(
    repoName: string,
    query?: AttestationsQuery
  ): Promise<PaginatedResponse<Attestation>> {
    const params = new URLSearchParams();
    if (query?.commitSha) params.set("commit_sha", query.commitSha);
    if (query?.changeId) params.set("change_id", query.changeId);
    if (query?.status) params.set("status", query.status);
    if (query?.limit) params.set("limit", String(query.limit));
    if (query?.offset) params.set("offset", String(query.offset));

    const queryString = params.toString();
    const path = `/${repoName}.jul/api/v1/attestations${queryString ? `?${queryString}` : ""}`;
    const data = await this.request<unknown>(path);
    return normalizePaginated(data, mapAttestation);
  }

  async triggerCI(
    repoName: string,
    data: { commitSha: string; profile?: "unit" | "full" | "lint" }
  ): Promise<{ jobId: string }> {
    const response = await this.request<unknown>(
      `/${repoName}.jul/api/v1/ci/trigger`,
      {
        method: "POST",
        body: JSON.stringify({
          commit_sha: data.commitSha,
          profile: data.profile,
        }),
      }
    );
    return mapJobResponse(response);
  }

  // === Suggestions ===

  async listSuggestions(
    repoName: string,
    query?: { changeId?: string; status?: Suggestion["status"] }
  ): Promise<Suggestion[]> {
    const params = new URLSearchParams();
    if (query?.changeId) params.set("change_id", query.changeId);
    if (query?.status) params.set("status", query.status);

    const queryString = params.toString();
    const path = `/${repoName}.jul/api/v1/suggestions${queryString ? `?${queryString}` : ""}`;
    const data = await this.request<unknown>(path);
    return mapArray(data, mapSuggestion);
  }

  async getSuggestion(repoName: string, suggestionId: string): Promise<Suggestion> {
    const data = await this.request<unknown>(
      `/${repoName}.jul/api/v1/suggestions/${suggestionId}`
    );
    return mapSuggestion(data);
  }

  async requestSuggestion(
    repoName: string,
    data: { changeId: string; reason: string }
  ): Promise<Suggestion> {
    const response = await this.request<unknown>(
      `/${repoName}.jul/api/v1/suggestions`,
      {
        method: "POST",
        body: JSON.stringify({
          change_id: data.changeId,
          reason: data.reason,
        }),
      }
    );
    return mapSuggestion(response);
  }

  async acceptSuggestion(repoName: string, suggestionId: string): Promise<void> {
    await this.request(`/${repoName}.jul/api/v1/suggestions/${suggestionId}/accept`, {
      method: "POST",
    });
  }

  async rejectSuggestion(repoName: string, suggestionId: string): Promise<void> {
    await this.request(`/${repoName}.jul/api/v1/suggestions/${suggestionId}/reject`, {
      method: "POST",
    });
  }

  // === Files ===

  async getFileTree(
    repoName: string,
    ref: string = "HEAD"
  ): Promise<FileNode[]> {
    return this.request<FileNode[]>(
      `/${repoName}.jul/api/v1/files?ref=${encodeURIComponent(ref)}`
    );
  }

  async getFileContent(
    repoName: string,
    path: string,
    ref: string = "HEAD"
  ): Promise<FileContent> {
    return this.request<FileContent>(
      `/${repoName}.jul/api/v1/files/${encodeURIComponent(path)}/content?ref=${encodeURIComponent(ref)}`
    );
  }

  async getFileHistory(
    repoName: string,
    path: string,
    options?: { ref?: string; limit?: number }
  ): Promise<FileHistoryEntry[]> {
    const params = new URLSearchParams();
    if (options?.ref) params.set("ref", options.ref);
    if (options?.limit) params.set("limit", String(options.limit));

    const queryString = params.toString();
    const data = await this.request<unknown>(
      `/${repoName}.jul/api/v1/files/${encodeURIComponent(path)}/history${queryString ? `?${queryString}` : ""}`
    );
    return mapArray(data, mapFileHistoryEntry);
  }

  // === Query ===

  async query(repoName: string, params: QueryParams): Promise<Commit[]> {
    const searchParams = new URLSearchParams();
    if (params.tests) searchParams.set("tests", params.tests);
    if (params.compiles !== undefined) searchParams.set("compiles", String(params.compiles));
    if (params.coverageMin) searchParams.set("coverage_min", String(params.coverageMin));
    if (params.coverageMax) searchParams.set("coverage_max", String(params.coverageMax));
    if (params.changeId) searchParams.set("change_id", params.changeId);
    if (params.author) searchParams.set("author", params.author);
    if (params.since) searchParams.set("since", params.since);
    if (params.until) searchParams.set("until", params.until);
    if (params.limit) searchParams.set("limit", String(params.limit));

    const queryString = searchParams.toString();
    const data = await this.request<unknown>(
      `/${repoName}.jul/api/v1/query${queryString ? `?${queryString}` : ""}`
    );
    return mapArray(data, mapCommit);
  }

  // === Events (SSE) ===

  subscribeToEvents(
    repoName: string,
    onEvent: (event: JulEvent) => void,
    options?: { since?: string }
  ): () => void {
    const params = new URLSearchParams();
    if (options?.since) params.set("since", options.since);

    const queryString = params.toString();
    const url = `${this.baseUrl}/${repoName}.jul/api/v1/events/stream${queryString ? `?${queryString}` : ""}`;

    const eventSource = new EventSource(url);

    eventSource.onmessage = (event) => {
      try {
        const data = mapJulEvent(JSON.parse(event.data) as JulEvent);
        onEvent(data);
      } catch (e) {
        console.error("Failed to parse event:", e);
      }
    };

    eventSource.onerror = (error) => {
      console.error("EventSource error:", error);
    };

    // Return cleanup function
    return () => {
      eventSource.close();
    };
  }
}

// Default client instance
export const julClient = new JulClient();

// React hook for using the client
export function useJulClient(config?: JulClientConfig): JulClient {
  // In a real app, this would be a React context provider
  // For now, return a new client or the default
  if (config) {
    return new JulClient(config);
  }
  return julClient;
}
