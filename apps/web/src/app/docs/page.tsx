import Link from "next/link";
import { Header, Footer } from "@/components/layout";

const docsSections = [
  {
    title: "Getting Started",
    items: [
      { title: "Introduction", href: "/docs/introduction", description: "What is Jul and why use it" },
      { title: "Quick Start", href: "/docs/quickstart", description: "Get up and running in 5 minutes" },
      { title: "Installation", href: "/docs/installation", description: "Install the Jul CLI" },
    ],
  },
  {
    title: "Core Concepts",
    items: [
      { title: "Sync-by-Default", href: "/docs/sync", description: "How automatic syncing works" },
      { title: "Changes & Revisions", href: "/docs/changes", description: "Understanding change-centric history" },
      { title: "Workspaces", href: "/docs/workspaces", description: "Multi-device workflow" },
      { title: "Attestations", href: "/docs/attestations", description: "CI results as first-class data" },
    ],
  },
  {
    title: "CLI Reference",
    items: [
      { title: "jul init", href: "/docs/cli/init", description: "Initialize a new repository" },
      { title: "jul sync", href: "/docs/cli/sync", description: "Sync your work to the server" },
      { title: "jul status", href: "/docs/cli/status", description: "View current state" },
      { title: "jul promote", href: "/docs/cli/promote", description: "Promote to a published branch" },
      { title: "jul query", href: "/docs/cli/query", description: "Query commits by criteria" },
    ],
  },
  {
    title: "API Reference",
    items: [
      { title: "Authentication", href: "/docs/api/auth", description: "API authentication" },
      { title: "Repositories", href: "/docs/api/repos", description: "Repository endpoints" },
      { title: "Changes", href: "/docs/api/changes", description: "Change management" },
      { title: "Attestations", href: "/docs/api/attestations", description: "CI and coverage data" },
      { title: "Events", href: "/docs/api/events", description: "Real-time event stream" },
    ],
  },
];

export default function DocsPage() {
  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="flex-1 pt-24 pb-16">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          {/* Header */}
          <div className="max-w-3xl">
            <h1 className="font-display text-4xl font-semibold tracking-tight">
              Documentation
            </h1>
            <p className="mt-4 text-lg text-muted-foreground">
              Learn how to use Jul for AI-first Git hosting with sync-by-default,
              change-centric history, and agent-native primitives.
            </p>
          </div>

          {/* Quick Start */}
          <div className="mt-12 rounded-xl border border-border bg-card/50 p-6">
            <h2 className="font-display text-lg font-semibold mb-4">Quick Start</h2>
            <div className="font-mono text-sm bg-muted/50 rounded-lg p-4 overflow-x-auto">
              <div className="space-y-2">
                <p><span className="text-muted-foreground"># Install the CLI</span></p>
                <p><span className="text-primary">brew</span> install jul</p>
                <p className="mt-4"><span className="text-muted-foreground"># Initialize a project</span></p>
                <p><span className="text-primary">jul</span> init my-project --server https://jul.example.com</p>
                <p className="mt-4"><span className="text-muted-foreground"># Your commits are now auto-synced!</span></p>
                <p><span className="text-primary">jul</span> commit -m <span className="text-chart-5">&quot;feat: add authentication&quot;</span></p>
              </div>
            </div>
          </div>

          {/* Documentation sections */}
          <div className="mt-16 grid gap-12 lg:grid-cols-2">
            {docsSections.map((section) => (
              <div key={section.title}>
                <h2 className="font-display text-xl font-semibold mb-6">
                  {section.title}
                </h2>
                <div className="space-y-4">
                  {section.items.map((item) => (
                    <Link
                      key={item.href}
                      href={item.href}
                      className="block group rounded-lg border border-border/50 bg-card/30 p-4 hover:border-primary/50 hover:bg-card transition-all"
                    >
                      <h3 className="font-medium group-hover:text-primary transition-colors">
                        {item.title}
                      </h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {item.description}
                      </p>
                    </Link>
                  ))}
                </div>
              </div>
            ))}
          </div>

          {/* Footer CTA */}
          <div className="mt-20 text-center">
            <p className="text-muted-foreground">
              Can&apos;t find what you&apos;re looking for?
            </p>
            <div className="mt-4 flex items-center justify-center gap-4">
              <Link
                href="https://github.com/jul-sh/jul/discussions"
                className="text-sm text-primary hover:underline"
              >
                Ask on GitHub Discussions
              </Link>
              <span className="text-muted-foreground">&#8226;</span>
              <Link
                href="/contact"
                className="text-sm text-primary hover:underline"
              >
                Contact Support
              </Link>
            </div>
          </div>
        </div>
      </main>
      <Footer />
    </div>
  );
}
