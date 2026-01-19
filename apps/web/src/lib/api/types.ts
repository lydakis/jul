// Jul API Types
// Based on the jul-spec.md design document

// === Core Entities ===

export interface Repo {
  id: string;
  name: string;
  description?: string;
  visibility: "public" | "private";
  defaultBranch: string;
  createdAt: string;
  updatedAt: string;
}

export interface Workspace {
  id: string;
  user: string;
  name: string;
  repoId: string;
  ref: string;
  headCommit: string;
  syncedAt: string;
}

export interface Change {
  changeId: string; // Format: Iab4f3c2d...
  title: string;
  author: string;
  createdAt: string;
  latestRevision: {
    revIndex: number;
    commitSha: string;
  };
  revisions: Revision[];
  status: "draft" | "ready" | "published" | "abandoned";
}

export interface Revision {
  revIndex: number;
  commitSha: string;
  createdAt: string;
}

export interface Commit {
  sha: string;
  changeId?: string;
  treeSha: string;
  author: string;
  authorEmail: string;
  message: string;
  createdAt: string;
}

// === Attestations ===

export type AttestationStatus = "running" | "pass" | "fail" | "error";
export type SignalStatus = "pass" | "fail" | "warn" | "complete";

export interface Signal {
  status: SignalStatus;
  message?: string;
}

export interface FormatSignal extends Signal {
  files?: string[];
}

export interface LintSignal extends Signal {
  warnings: number;
  errors: number;
}

export interface CompileSignal extends Signal {
  durationMs: number;
}

export interface TestSignal extends Signal {
  passed: number;
  failed: number;
  skipped: number;
  failures?: TestFailure[];
}

export interface TestFailure {
  name: string;
  file: string;
  line: number;
  message: string;
  stackTrace?: string;
}

export interface CoverageSignal extends Signal {
  linePct: number;
  branchPct: number;
  diffLinePct?: number;
  uncoveredLines?: Record<string, number[]>;
}

export interface Attestation {
  attestationId: string;
  commitSha: string;
  changeId?: string;
  type: "ci";
  status: AttestationStatus;
  signals: {
    format?: FormatSignal;
    lint?: LintSignal;
    compile?: CompileSignal;
    test?: TestSignal;
    coverage?: CoverageSignal;
  };
  artifacts?: Artifact[];
  logExcerpt?: string;
  startedAt: string;
  finishedAt?: string;
  createdAt: string;
}

export interface Artifact {
  name: string;
  uri: string;
}

// === Suggestions ===

export interface Suggestion {
  suggestionId: string;
  changeId: string;
  baseCommitSha: string;
  suggestedCommitSha: string;
  createdBy: string;
  createdAt: string;
  reason: string;
  description: string;
  confidence: number;
  status: "open" | "accepted" | "rejected" | "superseded";
  diffstat: {
    filesChanged: number;
    additions: number;
    deletions: number;
  };
}

// === Files ===

export interface FileNode {
  name: string;
  path: string;
  type: "file" | "directory";
  size?: number;
  children?: FileNode[];
}

export interface FileContent {
  path: string;
  content: string;
  encoding: "utf-8" | "base64";
  size: number;
  language?: string;
}

export interface FileHistoryEntry {
  commitSha: string;
  changeId?: string;
  author: string;
  message: string;
  createdAt: string;
  changeType: "add" | "modify" | "delete" | "rename";
}

// === Events ===

export type EventType =
  | "ref.updated"
  | "ci.started"
  | "ci.finished"
  | "attestation.added"
  | "suggestion.created"
  | "policy.violation";

export interface JulEvent {
  eventId: string;
  type: EventType;
  repo: string;
  ref?: string;
  commitSha?: string;
  changeId?: string;
  summary?: string;
  attestationId?: string;
  createdAt: string;
}

// === API Responses ===

export interface ApiError {
  error: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface PromoteResponse {
  success: boolean;
  ref: string;
  commitSha: string;
}

export interface PolicyViolation {
  check: string;
  status: "fail" | "warn";
  message: string;
}

export interface PromoteError extends ApiError {
  error: "policy_violation";
  violations: PolicyViolation[];
}

// === Query Parameters ===

export interface ChangesQuery {
  status?: Change["status"];
  author?: string;
  limit?: number;
  offset?: number;
}

export interface AttestationsQuery {
  commitSha?: string;
  changeId?: string;
  status?: AttestationStatus;
  limit?: number;
  offset?: number;
}

export interface QueryParams {
  tests?: "pass" | "fail";
  compiles?: boolean;
  coverageMin?: number;
  coverageMax?: number;
  changeId?: string;
  author?: string;
  since?: string;
  until?: string;
  limit?: number;
}
