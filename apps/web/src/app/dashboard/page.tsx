import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { RepoCard, CreateRepoDialog } from "@/components/dashboard";

// Mock data - will be replaced with API calls
const mockRepos = [
  {
    name: "jul",
    description: "AI-First Git Hosting - sync-by-default, change-centric history",
    language: "Go",
    visibility: "public" as const,
    updatedAt: "2h ago",
    syncStatus: "synced" as const,
    ciStatus: "passing" as const,
  },
  {
    name: "my-webapp",
    description: "A modern web application built with Next.js and Tailwind",
    language: "TypeScript",
    visibility: "private" as const,
    updatedAt: "5h ago",
    syncStatus: "synced" as const,
    ciStatus: "failing" as const,
  },
  {
    name: "ml-experiments",
    description: "Machine learning experiments and notebooks",
    language: "Python",
    visibility: "private" as const,
    updatedAt: "1d ago",
    syncStatus: "pending" as const,
    ciStatus: "none" as const,
  },
  {
    name: "dotfiles",
    description: "My personal configuration files",
    visibility: "public" as const,
    updatedAt: "3d ago",
    syncStatus: "synced" as const,
    ciStatus: "none" as const,
  },
];

export default function DashboardPage() {
  return (
    <div className="min-h-screen bg-background">
      {/* Dashboard Header */}
      <header className="sticky top-0 z-50 border-b border-border/40 bg-background/80 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-4">
            <a href="/" className="flex items-center gap-2">
              <span className="font-display text-xl font-semibold">jul</span>
              <span className="text-muted-foreground/50 text-lg font-light">줄</span>
            </a>
          </div>
          <div className="flex items-center gap-4">
            <div className="w-64">
              <Input
                type="search"
                placeholder="Search repositories..."
                className="h-9 bg-muted/50"
              />
            </div>
            <div className="h-8 w-8 rounded-full bg-primary/20 flex items-center justify-center text-sm font-medium">
              G
            </div>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        {/* Page Header */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="font-display text-2xl font-semibold">Repositories</h1>
            <p className="mt-1 text-muted-foreground">
              Your synced repositories and workspaces
            </p>
          </div>
          <CreateRepoDialog />
        </div>

        {/* Tabs */}
        <Tabs defaultValue="all" className="space-y-6">
          <div className="flex items-center justify-between">
            <TabsList>
              <TabsTrigger value="all">All</TabsTrigger>
              <TabsTrigger value="public">Public</TabsTrigger>
              <TabsTrigger value="private">Private</TabsTrigger>
            </TabsList>
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span>Sort by:</span>
              <select className="bg-transparent border-none text-foreground cursor-pointer focus:outline-none">
                <option>Last updated</option>
                <option>Name</option>
                <option>Created</option>
              </select>
            </div>
          </div>

          <TabsContent value="all" className="mt-0">
            <div className="grid gap-4 sm:grid-cols-2">
              {mockRepos.map((repo) => (
                <RepoCard key={repo.name} {...repo} />
              ))}
            </div>
          </TabsContent>

          <TabsContent value="public" className="mt-0">
            <div className="grid gap-4 sm:grid-cols-2">
              {mockRepos
                .filter((r) => r.visibility === "public")
                .map((repo) => (
                  <RepoCard key={repo.name} {...repo} />
                ))}
            </div>
          </TabsContent>

          <TabsContent value="private" className="mt-0">
            <div className="grid gap-4 sm:grid-cols-2">
              {mockRepos
                .filter((r) => r.visibility === "private")
                .map((repo) => (
                  <RepoCard key={repo.name} {...repo} />
                ))}
            </div>
          </TabsContent>
        </Tabs>

        {/* Quick Stats */}
        <div className="mt-12 grid gap-4 sm:grid-cols-3">
          <div className="rounded-xl border border-border/50 bg-card/50 p-6">
            <div className="text-3xl font-semibold text-primary">{mockRepos.length}</div>
            <div className="mt-1 text-sm text-muted-foreground">Total repositories</div>
          </div>
          <div className="rounded-xl border border-border/50 bg-card/50 p-6">
            <div className="text-3xl font-semibold text-chart-5">
              {mockRepos.filter((r) => r.syncStatus === "synced").length}
            </div>
            <div className="mt-1 text-sm text-muted-foreground">Synced</div>
          </div>
          <div className="rounded-xl border border-border/50 bg-card/50 p-6">
            <div className="text-3xl font-semibold text-chart-2">
              {mockRepos.filter((r) => r.ciStatus === "passing").length}
            </div>
            <div className="mt-1 text-sm text-muted-foreground">CI passing</div>
          </div>
        </div>
      </main>
    </div>
  );
}
