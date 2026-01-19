import Link from "next/link";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

interface RepoCardProps {
  name: string;
  description?: string;
  language?: string;
  visibility: "public" | "private";
  updatedAt: string;
  syncStatus: "synced" | "pending" | "error";
  ciStatus?: "passing" | "failing" | "running" | "none";
}

const languageColors: Record<string, string> = {
  TypeScript: "bg-blue-500",
  JavaScript: "bg-yellow-400",
  Python: "bg-green-500",
  Go: "bg-cyan-500",
  Rust: "bg-orange-500",
  default: "bg-muted-foreground",
};

export function RepoCard({
  name,
  description,
  language,
  visibility,
  updatedAt,
  syncStatus,
  ciStatus,
}: RepoCardProps) {
  return (
    <Link href={`/repo/${name}`}>
      <Card className="group h-full transition-all hover:border-primary/50 hover:bg-card/80">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-2">
            <CardTitle className="font-display text-lg group-hover:text-primary transition-colors">
              {name}
            </CardTitle>
            <Badge variant={visibility === "public" ? "secondary" : "outline"} className="text-xs">
              {visibility}
            </Badge>
          </div>
          {description && (
            <CardDescription className="line-clamp-2">{description}</CardDescription>
          )}
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between text-sm">
            <div className="flex items-center gap-4">
              {language && (
                <div className="flex items-center gap-1.5">
                  <span
                    className={`w-2.5 h-2.5 rounded-full ${languageColors[language] || languageColors.default}`}
                  />
                  <span className="text-muted-foreground">{language}</span>
                </div>
              )}
              {ciStatus && ciStatus !== "none" && (
                <div className="flex items-center gap-1.5">
                  {ciStatus === "passing" && (
                    <span className="text-chart-5">&#10003; CI</span>
                  )}
                  {ciStatus === "failing" && (
                    <span className="text-destructive">&#10007; CI</span>
                  )}
                  {ciStatus === "running" && (
                    <span className="text-chart-4">&#9679; CI</span>
                  )}
                </div>
              )}
            </div>
            <div className="flex items-center gap-2 text-muted-foreground">
              {syncStatus === "synced" && (
                <span className="text-chart-5 text-xs">&#10003; Synced</span>
              )}
              {syncStatus === "pending" && (
                <span className="text-chart-4 text-xs">&#8635; Syncing</span>
              )}
              {syncStatus === "error" && (
                <span className="text-destructive text-xs">&#10007; Sync error</span>
              )}
              <span className="text-xs">{updatedAt}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
