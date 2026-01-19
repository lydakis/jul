import Link from "next/link";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { RepoHeader, FileTree, CodeViewer } from "@/components/repo";

// Mock file tree
const mockFileTree = [
  {
    name: "apps",
    type: "directory" as const,
    path: "apps",
    children: [
      {
        name: "cli",
        type: "directory" as const,
        path: "apps/cli",
        children: [
          { name: "main.go", type: "file" as const, path: "apps/cli/main.go" },
          { name: "sync.go", type: "file" as const, path: "apps/cli/sync.go" },
          { name: "status.go", type: "file" as const, path: "apps/cli/status.go" },
        ],
      },
      {
        name: "server",
        type: "directory" as const,
        path: "apps/server",
        children: [
          { name: "main.go", type: "file" as const, path: "apps/server/main.go" },
          { name: "api.go", type: "file" as const, path: "apps/server/api.go" },
        ],
      },
    ],
  },
  {
    name: "docs",
    type: "directory" as const,
    path: "docs",
    children: [
      { name: "jul-spec.md", type: "file" as const, path: "docs/jul-spec.md" },
      { name: "README.md", type: "file" as const, path: "docs/README.md" },
    ],
  },
  { name: "README.md", type: "file" as const, path: "README.md" },
  { name: "go.mod", type: "file" as const, path: "go.mod" },
  { name: ".gitignore", type: "file" as const, path: ".gitignore" },
];

// Mock README content
const mockReadme = `# Jul - AI-First Git Hosting

**Sync-by-default. Change-centric history. Agent-native primitives.**

## What is Jul?

Jul (줄, Korean for "line") is a Git hosting platform designed for the age of AI coding agents.

## Features

- **Sync-by-Default**: Every commit is backed up immediately
- **Change-Centric History**: Stable identity across amend/rebase
- **First-Class Attestations**: CI/coverage/lint as queryable metadata
- **Agent-Native Queries**: "Last green revision," interdiff, structured context

## Quick Start

\`\`\`bash
# Install the CLI
brew install jul

# Initialize a new project
jul init my-project --server https://jul.example.com

# Your commits are now auto-synced!
jul commit -m "feat: add authentication"
\`\`\`

## License

MIT`;

// Mock changes
const mockChanges = [
  {
    id: "Iab4f3c2d",
    title: "feat: add user authentication",
    author: "george",
    status: "draft",
    revision: 3,
    updatedAt: "2h ago",
    ciStatus: "passing",
  },
  {
    id: "Icd5e6f7a",
    title: "refactor: extract utils package",
    author: "george",
    status: "ready",
    revision: 1,
    updatedAt: "1d ago",
    ciStatus: "passing",
  },
  {
    id: "Ief8a9b0c",
    title: "fix: handle edge case in sync",
    author: "george",
    status: "published",
    revision: 2,
    updatedAt: "3d ago",
    ciStatus: "passing",
  },
];

export default async function RepoPage({
  params,
}: {
  params: Promise<{ name: string }>;
}) {
  const { name } = await params;

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="sticky top-0 z-50 border-b border-border/40 bg-background/80 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-4">
            <Link href="/dashboard" className="flex items-center gap-2">
              <span className="font-display text-xl font-semibold">jul</span>
              <span className="text-muted-foreground/50 text-lg font-light">줄</span>
            </Link>
          </div>
          <div className="h-8 w-8 rounded-full bg-primary/20 flex items-center justify-center text-sm font-medium">
            G
          </div>
        </div>
      </header>

      {/* Repo Header */}
      <RepoHeader
        name={name}
        description="AI-First Git Hosting - sync-by-default, change-centric history"
        visibility="public"
        syncStatus="synced"
        ciStatus="passing"
        defaultBranch="main"
      />

      {/* Main Content */}
      <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        <Tabs defaultValue="code" className="space-y-6">
          <TabsList>
            <TabsTrigger value="code" className="gap-2">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" />
              </svg>
              Code
            </TabsTrigger>
            <TabsTrigger value="changes" className="gap-2">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 21 3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
              </svg>
              Changes
              <span className="ml-1 rounded-full bg-muted px-2 py-0.5 text-xs">
                {mockChanges.length}
              </span>
            </TabsTrigger>
            <TabsTrigger value="ci" className="gap-2">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75m-3-7.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285Z" />
              </svg>
              CI
            </TabsTrigger>
            <TabsTrigger value="settings" className="gap-2">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
              </svg>
              Settings
            </TabsTrigger>
          </TabsList>

          {/* Code Tab */}
          <TabsContent value="code" className="mt-0">
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-4">
              {/* File Tree */}
              <div className="lg:col-span-1">
                <div className="rounded-lg border border-border bg-card">
                  <div className="px-4 py-3 border-b border-border">
                    <h3 className="text-sm font-medium">Files</h3>
                  </div>
                  <ScrollArea className="h-[500px]">
                    <FileTree files={mockFileTree} repoName={name} />
                  </ScrollArea>
                </div>
              </div>

              {/* README */}
              <div className="lg:col-span-3">
                <CodeViewer code={mockReadme} language="markdown" filename="README.md" />
              </div>
            </div>
          </TabsContent>

          {/* Changes Tab */}
          <TabsContent value="changes" className="mt-0">
            <div className="space-y-4">
              {mockChanges.map((change) => (
                <div
                  key={change.id}
                  className="flex items-center justify-between rounded-lg border border-border bg-card p-4 hover:bg-card/80 transition-colors"
                >
                  <div className="flex items-center gap-4">
                    <div
                      className={`w-2 h-2 rounded-full ${
                        change.status === "published"
                          ? "bg-chart-5"
                          : change.status === "ready"
                          ? "bg-primary"
                          : "bg-muted-foreground"
                      }`}
                    />
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm text-accent">{change.id}</span>
                        <span className="font-medium">{change.title}</span>
                      </div>
                      <div className="flex items-center gap-3 mt-1 text-sm text-muted-foreground">
                        <span>{change.author}</span>
                        <span>&#8226;</span>
                        <span>rev {change.revision}</span>
                        <span>&#8226;</span>
                        <span>{change.updatedAt}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <span
                      className={`text-sm ${
                        change.ciStatus === "passing" ? "text-chart-5" : "text-destructive"
                      }`}
                    >
                      {change.ciStatus === "passing" ? "✓ CI" : "✗ CI"}
                    </span>
                    <span
                      className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${
                        change.status === "published"
                          ? "bg-chart-5/20 text-chart-5"
                          : change.status === "ready"
                          ? "bg-primary/20 text-primary"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      {change.status}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </TabsContent>

          {/* CI Tab */}
          <TabsContent value="ci" className="mt-0">
            <div className="rounded-lg border border-border bg-card p-6">
              <div className="flex items-center gap-3 mb-6">
                <div className="flex items-center justify-center w-10 h-10 rounded-full bg-chart-5/20">
                  <svg className="w-5 h-5 text-chart-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 12.75 6 6 9-13.5" />
                  </svg>
                </div>
                <div>
                  <h3 className="font-medium">All checks passing</h3>
                  <p className="text-sm text-muted-foreground">Last run 2 hours ago</p>
                </div>
              </div>

              <div className="space-y-3">
                {[
                  { name: "Format", status: "pass", time: "2s" },
                  { name: "Lint", status: "pass", time: "5s", warnings: 2 },
                  { name: "Build", status: "pass", time: "12s" },
                  { name: "Test", status: "pass", time: "45s", details: "47 passed" },
                  { name: "Coverage", status: "pass", time: "3s", details: "84.1%" },
                ].map((check) => (
                  <div
                    key={check.name}
                    className="flex items-center justify-between py-2 border-b border-border/50 last:border-0"
                  >
                    <div className="flex items-center gap-3">
                      <span className="text-chart-5">&#10003;</span>
                      <span className="font-medium">{check.name}</span>
                      {check.details && (
                        <span className="text-sm text-muted-foreground">{check.details}</span>
                      )}
                      {check.warnings && (
                        <span className="text-sm text-chart-4">{check.warnings} warnings</span>
                      )}
                    </div>
                    <span className="text-sm text-muted-foreground">{check.time}</span>
                  </div>
                ))}
              </div>
            </div>
          </TabsContent>

          {/* Settings Tab */}
          <TabsContent value="settings" className="mt-0">
            <div className="rounded-lg border border-border bg-card p-6">
              <h3 className="font-display text-lg font-semibold mb-4">Repository Settings</h3>
              <p className="text-muted-foreground">Settings page coming soon...</p>
            </div>
          </TabsContent>
        </Tabs>
      </main>
    </div>
  );
}
