import { codeToHtml, type BundledLanguage } from "shiki";
import { ScrollArea } from "@/components/ui/scroll-area";

interface CodeViewerProps {
  code: string;
  language: string;
  filename: string;
}

// Map common file extensions to Shiki language names
const languageMap: Record<string, BundledLanguage> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  py: "python",
  go: "go",
  rs: "rust",
  md: "markdown",
  mdx: "mdx",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "toml",
  css: "css",
  scss: "scss",
  html: "html",
  sql: "sql",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  dockerfile: "dockerfile",
  graphql: "graphql",
  proto: "protobuf",
};

function getLanguage(filename: string, providedLang: string): BundledLanguage {
  // Check provided language first
  if (providedLang && languageMap[providedLang.toLowerCase()]) {
    return languageMap[providedLang.toLowerCase()];
  }

  // Fall back to extension
  const ext = filename.split(".").pop()?.toLowerCase() || "";
  return languageMap[ext] || "text";
}

export async function CodeViewer({ code, language, filename }: CodeViewerProps) {
  const lines = code.split("\n");
  const lang = getLanguage(filename, language);

  let highlightedHtml: string;
  try {
    highlightedHtml = await codeToHtml(code, {
      lang,
      theme: "github-dark-default",
    });
  } catch {
    // Fallback to plain text if highlighting fails
    highlightedHtml = `<pre><code>${code.replace(/</g, "&lt;").replace(/>/g, "&gt;")}</code></pre>`;
  }

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      {/* File header */}
      <div className="flex items-center justify-between px-4 py-2 bg-muted/50 border-b border-border">
        <div className="flex items-center gap-2">
          <svg
            className="w-4 h-4 text-muted-foreground"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={1.5}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z"
            />
          </svg>
          <span className="text-sm font-medium">{filename}</span>
          <span className="text-xs text-muted-foreground px-1.5 py-0.5 bg-muted rounded">
            {lang}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">{lines.length} lines</span>
          <button
            className="p-1.5 hover:bg-muted rounded transition-colors"
            title="Copy to clipboard"
          >
            <svg
              className="w-4 h-4 text-muted-foreground"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M15.666 3.888A2.25 2.25 0 0 0 13.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 0 1-.75.75H9a.75.75 0 0 1-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 0 1-2.25 2.25H6.75A2.25 2.25 0 0 1 4.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 0 1 1.927-.184"
              />
            </svg>
          </button>
          <button
            className="p-1.5 hover:bg-muted rounded transition-colors"
            title="Raw file"
          >
            <svg
              className="w-4 h-4 text-muted-foreground"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5"
              />
            </svg>
          </button>
        </div>
      </div>

      {/* Code content with Shiki highlighting */}
      <ScrollArea className="max-h-[600px]">
        <div className="relative">
          {/* Line numbers */}
          <div className="absolute left-0 top-0 bottom-0 w-12 bg-card/50 border-r border-border/50 select-none pointer-events-none z-10">
            <div className="py-4 text-right pr-3 font-mono text-sm leading-6">
              {lines.map((_, index) => (
                <div key={index} className="text-muted-foreground/50">
                  {index + 1}
                </div>
              ))}
            </div>
          </div>

          {/* Highlighted code */}
          <div
            className="pl-14 [&_pre]:!bg-transparent [&_pre]:!p-4 [&_pre]:!m-0 [&_code]:!bg-transparent [&_.line]:leading-6"
            dangerouslySetInnerHTML={{ __html: highlightedHtml }}
          />
        </div>
      </ScrollArea>
    </div>
  );
}
