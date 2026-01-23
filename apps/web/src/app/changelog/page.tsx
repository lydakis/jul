import Link from "next/link";
import { Header, Footer } from "@/components/layout";
import { Badge } from "@/components/ui/badge";

const changelog = [
  {
    version: "0.3.0",
    date: "January 15, 2026",
    type: "feature",
    title: "AI-Powered Suggestions",
    description:
      "Jul can now suggest fixes when your tests fail. Enable in your repository settings to have the server analyze failures and propose patches.",
    changes: [
      "Added suggestion API endpoints",
      "New `jul suggestions` and `jul apply` commands",
      "Confidence scoring for suggestions",
      "Support for custom AI providers",
    ],
  },
  {
    version: "0.2.0",
    date: "December 20, 2025",
    type: "feature",
    title: "Change-Centric History",
    description:
      "Introducing stable Change-IDs that survive amend and rebase. Track the evolution of your work across rewrites.",
    changes: [
      "Change-Id generation and tracking",
      "Interdiff support for comparing revisions",
      "New `jul changes` and `jul interdiff` commands",
      "Gerrit Change-Id compatibility",
      "JJ change-id header support",
    ],
  },
  {
    version: "0.1.2",
    date: "December 10, 2025",
    type: "fix",
    title: "Sync Reliability Improvements",
    description:
      "Fixed several edge cases in the sync mechanism and improved error messages.",
    changes: [
      "Fixed race condition in concurrent syncs",
      "Better handling of network interruptions",
      "Improved error messages for auth failures",
      "Fixed keep-ref cleanup timing",
    ],
  },
  {
    version: "0.1.1",
    date: "December 1, 2025",
    type: "improvement",
    title: "CI Pipeline Enhancements",
    description: "Faster CI runs and better coverage reporting.",
    changes: [
      "Parallel test execution",
      "Incremental coverage reporting",
      "Support for custom CI profiles",
      "Artifact retention policies",
    ],
  },
  {
    version: "0.1.0",
    date: "November 15, 2025",
    type: "release",
    title: "Initial Beta Release",
    description:
      "The first public beta of Jul! Sync-by-default Git hosting with first-class attestations.",
    changes: [
      "Core sync-by-default functionality",
      "Workspace refs for automatic backup",
      "Basic CI with build, test, and coverage",
      "Jul CLI with init, sync, status, and promote",
      "Web dashboard for repository management",
      "SSE event stream for real-time updates",
    ],
  },
];

const typeColors: Record<string, string> = {
  feature: "bg-primary/20 text-primary",
  fix: "bg-destructive/20 text-destructive",
  improvement: "bg-accent/20 text-accent",
  release: "bg-chart-5/20 text-chart-5",
};

export default function ChangelogPage() {
  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="flex-1 pt-24 pb-16">
        <div className="mx-auto max-w-3xl px-4 sm:px-6 lg:px-8">
          {/* Header */}
          <div>
            <h1 className="font-display text-4xl font-semibold tracking-tight">
              Changelog
            </h1>
            <p className="mt-4 text-lg text-muted-foreground">
              New features, improvements, and fixes in Jul
            </p>
            <div className="mt-4 flex items-center gap-4">
              <Link
                href="/changelog/rss"
                className="text-sm text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
              >
                <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M6.18 15.64a2.18 2.18 0 0 1 2.18 2.18C8.36 19 7.38 20 6.18 20C5 20 4 19 4 17.82a2.18 2.18 0 0 1 2.18-2.18M4 4.44A15.56 15.56 0 0 1 19.56 20h-2.83A12.73 12.73 0 0 0 4 7.27V4.44m0 5.66a9.9 9.9 0 0 1 9.9 9.9h-2.83A7.07 7.07 0 0 0 4 12.93V10.1Z" />
                </svg>
                RSS Feed
              </Link>
              <Link
                href="https://github.com/jul-sh/jul/releases"
                className="text-sm text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
              >
                <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                </svg>
                GitHub Releases
              </Link>
            </div>
          </div>

          {/* Timeline */}
          <div className="mt-16 space-y-12">
            {changelog.map((entry, index) => (
              <article
                key={entry.version}
                className="relative pl-8 border-l border-border"
              >
                {/* Timeline dot */}
                <div className="absolute left-0 top-0 -translate-x-1/2 w-3 h-3 rounded-full bg-primary border-4 border-background" />

                {/* Content */}
                <div>
                  <div className="flex items-center gap-3 flex-wrap">
                    <Badge variant="outline" className="font-mono">
                      v{entry.version}
                    </Badge>
                    <Badge className={typeColors[entry.type]}>
                      {entry.type}
                    </Badge>
                    <span className="text-sm text-muted-foreground">
                      {entry.date}
                    </span>
                  </div>

                  <h2 className="font-display text-xl font-semibold mt-4">
                    {entry.title}
                  </h2>
                  <p className="mt-2 text-muted-foreground">{entry.description}</p>

                  <ul className="mt-4 space-y-2">
                    {entry.changes.map((change) => (
                      <li key={change} className="flex items-start gap-2 text-sm">
                        <span className="text-primary mt-1">&#8226;</span>
                        <span>{change}</span>
                      </li>
                    ))}
                  </ul>
                </div>

                {/* Connector line extension for last item */}
                {index === changelog.length - 1 && (
                  <div className="absolute left-0 bottom-0 top-6 -translate-x-1/2 w-0.5 bg-gradient-to-b from-border to-transparent" />
                )}
              </article>
            ))}
          </div>

          {/* Subscribe */}
          <div className="mt-20 rounded-xl border border-border bg-card/50 p-8 text-center">
            <h2 className="font-display text-xl font-semibold">
              Stay up to date
            </h2>
            <p className="mt-2 text-muted-foreground">
              Get notified when we release new features
            </p>
            <form className="mt-6 flex gap-2 max-w-md mx-auto">
              <input
                type="email"
                placeholder="you@example.com"
                className="flex-1 rounded-lg border border-border bg-background px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary/50"
              />
              <button
                type="submit"
                className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                Subscribe
              </button>
            </form>
          </div>
        </div>
      </main>
      <Footer />
    </div>
  );
}
