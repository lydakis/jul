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

// Configuration
const DEFAULT_BASE_URL = process.env.NEXT_PUBLIC_JUL_API_URL || "http://localhost:8000";

export interface JulClientConfig {
  baseUrl?: string;
  token?: string;
}

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
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const headers: HeadersInit = {
      "Content-Type": "application/json",
      ...options.headers,
    };

    if (this.token) {
      (headers as Record<string, string>)["Authorization"] = `Bearer ${this.token}`;
    }

    const response = await fetch(url, {
      ...options,
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
    if (!text) return {} as T;

    return JSON.parse(text);
  }

  // === Repositories ===

  async listRepos(): Promise<Repo[]> {
    return this.request<Repo[]>("/api/v1/repos");
  }

  async getRepo(name: string): Promise<Repo> {
    return this.request<Repo>(`/api/v1/repos/${name}`);
  }

  async createRepo(data: {
    name: string;
    description?: string;
    visibility?: "public" | "private";
  }): Promise<Repo> {
    return this.request<Repo>("/api/v1/repos", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteRepo(name: string): Promise<void> {
    await this.request(`/api/v1/repos/${name}`, {
      method: "DELETE",
    });
  }

  // === Workspaces ===

  async listWorkspaces(repoName: string): Promise<Workspace[]> {
    return this.request<Workspace[]>(`/${repoName}.jul/api/v1/workspaces`);
  }

  async getWorkspace(repoName: string, workspaceId: string): Promise<Workspace> {
    return this.request<Workspace>(
      `/${repoName}.jul/api/v1/workspaces/${workspaceId}`
    );
  }

  async promote(
    repoName: string,
    workspaceId: string,
    data: { targetBranch: string; commitSha?: string }
  ): Promise<PromoteResponse> {
    return this.request<PromoteResponse>(
      `/${repoName}.jul/api/v1/workspaces/${workspaceId}/promote`,
      {
        method: "POST",
        body: JSON.stringify({
          target_branch: data.targetBranch,
          commit_sha: data.commitSha,
        }),
      }
    );
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
    return this.request<PaginatedResponse<Change>>(path);
  }

  async getChange(repoName: string, changeId: string): Promise<Change> {
    return this.request<Change>(`/${repoName}.jul/api/v1/changes/${changeId}`);
  }

  async getInterdiff(
    repoName: string,
    changeId: string,
    fromRev: number,
    toRev: number
  ): Promise<string> {
    return this.request<string>(
      `/${repoName}.jul/api/v1/changes/${changeId}/interdiff?from_rev=${fromRev}&to_rev=${toRev}`
    );
  }

  // === Commits & Attestations ===

  async getCommit(repoName: string, sha: string): Promise<Commit> {
    return this.request<Commit>(`/${repoName}.jul/api/v1/commits/${sha}`);
  }

  async getAttestation(repoName: string, sha: string): Promise<Attestation> {
    return this.request<Attestation>(
      `/${repoName}.jul/api/v1/commits/${sha}/attestation`
    );
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
    return this.request<PaginatedResponse<Attestation>>(path);
  }

  async triggerCI(
    repoName: string,
    data: { commitSha: string; profile?: "unit" | "full" | "lint" }
  ): Promise<{ jobId: string }> {
    return this.request<{ jobId: string }>(`/${repoName}.jul/api/v1/ci/trigger`, {
      method: "POST",
      body: JSON.stringify({
        commit_sha: data.commitSha,
        profile: data.profile,
      }),
    });
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
    return this.request<Suggestion[]>(path);
  }

  async getSuggestion(repoName: string, suggestionId: string): Promise<Suggestion> {
    return this.request<Suggestion>(
      `/${repoName}.jul/api/v1/suggestions/${suggestionId}`
    );
  }

  async requestSuggestion(
    repoName: string,
    data: { changeId: string; reason: string }
  ): Promise<Suggestion> {
    return this.request<Suggestion>(`/${repoName}.jul/api/v1/suggestions`, {
      method: "POST",
      body: JSON.stringify({
        change_id: data.changeId,
        reason: data.reason,
      }),
    });
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
    return this.request<FileHistoryEntry[]>(
      `/${repoName}.jul/api/v1/files/${encodeURIComponent(path)}/history${queryString ? `?${queryString}` : ""}`
    );
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
    return this.request<Commit[]>(
      `/${repoName}.jul/api/v1/query${queryString ? `?${queryString}` : ""}`
    );
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
        const data = JSON.parse(event.data) as JulEvent;
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
