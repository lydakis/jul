import Link from "next/link";
import { RepoHeader, FileTree, CodeViewer } from "@/components/repo";
import { ScrollArea } from "@/components/ui/scroll-area";
import { julClient } from "@/lib/api";

export default async function RepoBlobPage({
  params,
}: {
  params: Promise<{ name: string; path?: string[] }>;
}) {
  const { name, path } = await params;
  const filePath = (path ?? []).join("/");
  const [fileTreeResult, fileContentResult] = await Promise.allSettled([
    julClient.getFileTree(name),
    julClient.getFileContent(name, filePath),
  ]);

  const fileTree =
    fileTreeResult.status === "fulfilled" ? fileTreeResult.value : [];
  const fileContent =
    fileContentResult.status === "fulfilled" ? fileContentResult.value : null;
  const fileError =
    fileContentResult.status === "rejected"
      ? "Unable to load this file yet."
      : null;

  let renderedContent = "";
  if (fileContent) {
    renderedContent =
      fileContent.encoding === "base64"
        ? Buffer.from(fileContent.content, "base64").toString("utf8")
        : fileContent.content;
  }

  const filename = filePath.split("/").pop() || filePath;

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-50 border-b border-border/40 bg-background/80 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <div className="flex items-center gap-4">
            <Link href="/dashboard" className="flex items-center gap-2">
              <span className="font-display text-xl font-semibold">jul</span>
            </Link>
          </div>
          <div className="h-8 w-8 rounded-full bg-primary/20 flex items-center justify-center text-sm font-medium">
            G
          </div>
        </div>
      </header>

      <RepoHeader
        name={name}
        description="AI-First Git Hosting - sync-by-default, change-centric history"
        visibility="public"
        syncStatus="synced"
        ciStatus="passing"
        defaultBranch="main"
      />

      <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        <div className="mb-4 flex items-center justify-between gap-4">
          <div>
            <h2 className="font-display text-lg font-semibold">File view</h2>
            <p className="text-sm text-muted-foreground">
              {filePath || "Unknown path"}
            </p>
          </div>
          <Link href={`/repo/${name}`} className="text-sm text-primary hover:underline">
            Back to repo
          </Link>
        </div>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-4">
          <div className="lg:col-span-1">
            <div className="rounded-lg border border-border bg-card">
              <div className="px-4 py-3 border-b border-border">
                <h3 className="text-sm font-medium">Files</h3>
              </div>
              <ScrollArea className="h-[500px]">
                <FileTree files={fileTree} repoName={name} currentPath={filePath} />
              </ScrollArea>
            </div>
          </div>

          <div className="lg:col-span-3">
            {fileContent ? (
              <CodeViewer
                code={renderedContent}
                language={fileContent.language ?? ""}
                filename={filename}
              />
            ) : (
              <div className="rounded-lg border border-border bg-card p-6 text-muted-foreground">
                {fileError ?? "Select a file to view its contents."}
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}
