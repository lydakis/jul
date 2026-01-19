import type {
  Attestation,
  Change,
  Commit,
  FileHistoryEntry,
  JulEvent,
  PaginatedResponse,
  PromoteResponse,
  Repo,
  Suggestion,
  Workspace,
} from "./types";

type AnyRecord = Record<string, any>;

type Mapper<T> = (input: unknown) => T;

export const mapArray = <T>(input: unknown, mapItem: Mapper<T>): T[] =>
  Array.isArray(input) ? input.map(mapItem) : [];

export const normalizePaginated = <T>(
  input: unknown,
  mapItem: Mapper<T>
): PaginatedResponse<T> => {
  if (Array.isArray(input)) {
    const items = input.map(mapItem);
    return {
      items,
      total: items.length,
      limit: items.length,
      offset: 0,
    };
  }

  const data = (input ?? {}) as AnyRecord;
  const rawItems = Array.isArray(data.items) ? data.items : [];
  const items = rawItems.map(mapItem);

  return {
    items,
    total: typeof data.total === "number" ? data.total : items.length,
    limit: typeof data.limit === "number" ? data.limit : items.length,
    offset: typeof data.offset === "number" ? data.offset : 0,
  };
};

export const mapRepo = (input: unknown): Repo => {
  const repo = input as AnyRecord;
  return {
    id: repo.id,
    name: repo.name,
    description: repo.description ?? undefined,
    visibility: repo.visibility,
    defaultBranch: repo.default_branch ?? repo.defaultBranch,
    createdAt: repo.created_at ?? repo.createdAt,
    updatedAt: repo.updated_at ?? repo.updatedAt,
  };
};

export const mapWorkspace = (input: unknown): Workspace => {
  const workspace = input as AnyRecord;
  return {
    id: workspace.id,
    user: workspace.user,
    name: workspace.name,
    repoId: workspace.repo_id ?? workspace.repoId,
    ref: workspace.ref,
    headCommit: workspace.head_commit ?? workspace.headCommit,
    syncedAt: workspace.synced_at ?? workspace.syncedAt,
  };
};

const mapRevision = (input: unknown) => {
  const revision = (input ?? {}) as AnyRecord;
  return {
    revIndex: revision.rev_index ?? revision.revIndex,
    commitSha: revision.commit_sha ?? revision.commitSha,
    createdAt: revision.created_at ?? revision.createdAt,
  };
};

export const mapChange = (input: unknown): Change => {
  const change = input as AnyRecord;
  const latestRevision = change.latest_revision ?? change.latestRevision;
  return {
    changeId: change.change_id ?? change.changeId,
    title: change.title,
    author: change.author,
    createdAt: change.created_at ?? change.createdAt,
    latestRevision: mapRevision(latestRevision),
    revisions: mapArray(change.revisions, mapRevision),
    status: change.status,
  };
};

export const mapCommit = (input: unknown): Commit => {
  const commit = input as AnyRecord;
  return {
    sha: commit.sha,
    changeId: commit.change_id ?? commit.changeId,
    treeSha: commit.tree_sha ?? commit.treeSha,
    author: commit.author,
    authorEmail: commit.author_email ?? commit.authorEmail,
    message: commit.message,
    createdAt: commit.created_at ?? commit.createdAt,
  };
};

const mapArtifact = (input: AnyRecord) => ({
  name: input.name,
  uri: input.uri,
});

const mapTestFailure = (input: AnyRecord) => ({
  name: input.name,
  file: input.file,
  line: input.line,
  message: input.message,
  stackTrace: input.stack_trace ?? input.stackTrace,
});

export const mapAttestation = (input: unknown): Attestation => {
  const attestation = input as AnyRecord;
  const signals = (attestation.signals ?? {}) as AnyRecord;
  return {
    attestationId: attestation.attestation_id ?? attestation.attestationId,
    commitSha: attestation.commit_sha ?? attestation.commitSha,
    changeId: attestation.change_id ?? attestation.changeId,
    type: attestation.type,
    status: attestation.status,
    signals: {
      format: signals.format
        ? {
            status: signals.format.status,
            message: signals.format.message,
            files: signals.format.files,
          }
        : undefined,
      lint: signals.lint
        ? {
            status: signals.lint.status,
            message: signals.lint.message,
            warnings: signals.lint.warnings,
            errors: signals.lint.errors,
          }
        : undefined,
      compile: signals.compile
        ? {
            status: signals.compile.status,
            message: signals.compile.message,
            durationMs: signals.compile.duration_ms ?? signals.compile.durationMs,
          }
        : undefined,
      test: signals.test
        ? {
            status: signals.test.status,
            message: signals.test.message,
            passed: signals.test.passed,
            failed: signals.test.failed,
            skipped: signals.test.skipped,
            failures: mapArray(signals.test.failures, mapTestFailure),
          }
        : undefined,
      coverage: signals.coverage
        ? {
            status: signals.coverage.status,
            message: signals.coverage.message,
            linePct: signals.coverage.line_pct ?? signals.coverage.linePct,
            branchPct: signals.coverage.branch_pct ?? signals.coverage.branchPct,
            diffLinePct: signals.coverage.diff_line_pct ?? signals.coverage.diffLinePct,
            uncoveredLines:
              signals.coverage.uncovered_lines ?? signals.coverage.uncoveredLines,
          }
        : undefined,
    },
    artifacts: mapArray(attestation.artifacts, mapArtifact),
    logExcerpt: attestation.log_excerpt ?? attestation.logExcerpt,
    startedAt: attestation.started_at ?? attestation.startedAt,
    finishedAt: attestation.finished_at ?? attestation.finishedAt,
    createdAt: attestation.created_at ?? attestation.createdAt,
  };
};

export const mapSuggestion = (input: unknown): Suggestion => {
  const suggestion = input as AnyRecord;
  const diffstat = (suggestion.diffstat ?? {}) as AnyRecord;
  return {
    suggestionId: suggestion.suggestion_id ?? suggestion.suggestionId,
    changeId: suggestion.change_id ?? suggestion.changeId,
    baseCommitSha: suggestion.base_commit_sha ?? suggestion.baseCommitSha,
    suggestedCommitSha:
      suggestion.suggested_commit_sha ?? suggestion.suggestedCommitSha,
    createdBy: suggestion.created_by ?? suggestion.createdBy,
    createdAt: suggestion.created_at ?? suggestion.createdAt,
    reason: suggestion.reason,
    description: suggestion.description,
    confidence: suggestion.confidence,
    status: suggestion.status,
    diffstat: {
      filesChanged: diffstat.files_changed ?? diffstat.filesChanged,
      additions: diffstat.additions,
      deletions: diffstat.deletions,
    },
  };
};

export const mapFileHistoryEntry = (input: unknown): FileHistoryEntry => {
  const entry = input as AnyRecord;
  return {
    commitSha: entry.commit_sha ?? entry.commitSha,
    changeId: entry.change_id ?? entry.changeId,
    author: entry.author,
    message: entry.message,
    createdAt: entry.created_at ?? entry.createdAt,
    changeType: entry.change_type ?? entry.changeType,
  };
};

export const mapJulEvent = (input: unknown): JulEvent => {
  const event = input as AnyRecord;
  return {
    eventId: event.event_id ?? event.eventId,
    type: event.type,
    repo: event.repo,
    ref: event.ref,
    commitSha: event.commit_sha ?? event.commitSha,
    changeId: event.change_id ?? event.changeId,
    summary: event.summary,
    attestationId: event.attestation_id ?? event.attestationId,
    createdAt: event.created_at ?? event.createdAt,
  };
};

export const mapPromoteResponse = (input: unknown): PromoteResponse => {
  const response = input as AnyRecord;
  return {
    success: response.success,
    ref: response.ref,
    commitSha: response.commit_sha ?? response.commitSha,
  };
};

export const mapJobResponse = (input: unknown): { jobId: string } => {
  const response = input as AnyRecord;
  return {
    jobId: response.job_id ?? response.jobId,
  };
};
