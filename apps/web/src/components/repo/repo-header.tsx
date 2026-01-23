import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

interface RepoHeaderProps {
  name: string;
  description?: string;
  visibility: "public" | "private";
  syncStatus: "synced" | "pending" | "error";
  ciStatus?: "passing" | "failing" | "running";
  defaultBranch: string;
}

export function RepoHeader({
  name,
  description,
  visibility,
  syncStatus,
  ciStatus,
  defaultBranch,
}: RepoHeaderProps) {
  return (
    <div className="border-b border-border/40 bg-background">
      <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        {/* Breadcrumb */}
        <div className="flex items-center gap-2 text-sm text-muted-foreground mb-4">
          <Link href="/dashboard" className="hover:text-foreground transition-colors">
            Dashboard
          </Link>
          <span>/</span>
          <span className="text-foreground font-medium">{name}</span>
        </div>

        {/* Title row */}
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-3 flex-wrap">
              <h1 className="font-display text-2xl font-semibold truncate">{name}</h1>
              <Badge variant={visibility === "public" ? "secondary" : "outline"}>
                {visibility}
              </Badge>
              {syncStatus === "synced" && (
                <Badge variant="outline" className="text-chart-5 border-chart-5/30">
                  &#10003; Synced
                </Badge>
              )}
              {syncStatus === "pending" && (
                <Badge variant="outline" className="text-chart-4 border-chart-4/30">
                  &#8635; Syncing
                </Badge>
              )}
              {ciStatus === "passing" && (
                <Badge variant="outline" className="text-chart-5 border-chart-5/30">
                  &#10003; CI Passing
                </Badge>
              )}
              {ciStatus === "failing" && (
                <Badge variant="outline" className="text-destructive border-destructive/30">
                  &#10007; CI Failing
                </Badge>
              )}
            </div>
            {description && (
              <p className="mt-2 text-muted-foreground">{description}</p>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm">
              <svg className="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M11.48 3.499a.562.562 0 0 1 1.04 0l2.125 5.111a.563.563 0 0 0 .475.345l5.518.442c.499.04.701.663.321.988l-4.204 3.602a.563.563 0 0 0-.182.557l1.285 5.385a.562.562 0 0 1-.84.61l-4.725-2.885a.562.562 0 0 0-.586 0L6.982 20.54a.562.562 0 0 1-.84-.61l1.285-5.386a.562.562 0 0 0-.182-.557l-4.204-3.602a.562.562 0 0 1 .321-.988l5.518-.442a.563.563 0 0 0 .475-.345L11.48 3.5Z" />
              </svg>
              Star
            </Button>
            <Button variant="outline" size="sm">
              <svg className="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M7.217 10.907a2.25 2.25 0 1 0 0 2.186m0-2.186c.18.324.283.696.283 1.093s-.103.77-.283 1.093m0-2.186 9.566-5.314m-9.566 7.5 9.566 5.314m0 0a2.25 2.25 0 1 0 3.935 2.186 2.25 2.25 0 0 0-3.935-2.186Zm0-12.814a2.25 2.25 0 1 0 3.933-2.185 2.25 2.25 0 0 0-3.933 2.185Z" />
              </svg>
              Fork
            </Button>
          </div>
        </div>

        {/* Clone URL */}
        <div className="mt-4 flex items-center gap-4">
          <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-1.5">
            <svg className="w-4 h-4 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 0 1 1.242 7.244l-4.5 4.5a4.5 4.5 0 0 1-6.364-6.364l1.757-1.757m13.35-.622 1.757-1.757a4.5 4.5 0 0 0-6.364-6.364l-4.5 4.5a4.5 4.5 0 0 0 1.242 7.244" />
            </svg>
            <code className="text-sm font-mono text-muted-foreground">
              jul clone {name}
            </code>
            <button className="ml-2 p-1 hover:bg-muted rounded transition-colors" title="Copy">
              <svg className="w-3.5 h-3.5 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M15.666 3.888A2.25 2.25 0 0 0 13.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 0 1-.75.75H9a.75.75 0 0 1-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 0 1-2.25 2.25H6.75A2.25 2.25 0 0 1 4.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 0 1 1.927-.184" />
              </svg>
            </button>
          </div>
          <span className="text-sm text-muted-foreground">
            Default branch: <span className="font-medium text-foreground">{defaultBranch}</span>
          </span>
        </div>
      </div>
    </div>
  );
}
