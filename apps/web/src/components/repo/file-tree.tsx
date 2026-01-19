"use client";

import { useState } from "react";
import Link from "next/link";

interface FileNode {
  name: string;
  type: "file" | "directory";
  path: string;
  children?: FileNode[];
}

interface FileTreeProps {
  files: FileNode[];
  repoName: string;
  currentPath?: string;
}

function FileIcon({ type, name }: { type: "file" | "directory"; name: string }) {
  if (type === "directory") {
    return (
      <svg className="w-4 h-4 text-chart-4" fill="currentColor" viewBox="0 0 20 20">
        <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
      </svg>
    );
  }

  // File type icons based on extension
  const ext = name.split(".").pop()?.toLowerCase();

  if (["ts", "tsx", "js", "jsx"].includes(ext || "")) {
    return (
      <svg className="w-4 h-4 text-blue-400" viewBox="0 0 24 24" fill="currentColor">
        <path d="M3 3h18v18H3V3zm16.525 13.707c-.131-.821-.666-1.511-2.252-2.155-.552-.259-1.165-.438-1.349-.854-.068-.248-.083-.382-.068-.54.025-.306.209-.475.518-.553.214-.054.431-.033.586.078.185.133.302.369.369.67.995-.597.995-.597 1.69-1-.256-.367-.382-.532-.552-.67-.608-.494-1.424-.669-2.256-.514-.83.163-1.517.561-1.867 1.25-.706 1.463-.159 3.575 1.412 4.507 1.56.932 2.242.818 2.647 1.582.116.223.163.525.073.786-.179.521-.648.733-1.217.688-.65-.05-1.025-.34-1.434-.969l-1.6.87c.152.37.34.582.552.796.806.798 1.899.986 3.024.79 1.02-.191 1.918-.862 2.236-1.785.418-1.214.178-2.486-.512-3.277zM10.72 12.83H8.1v5.18h2.062v-3.422c-.006-.364.154-.722.453-.936.309-.22.722-.225 1.032-.024.322.197.47.553.489.933v3.449h2.07v-3.822c0-1.3-.687-2.323-1.994-2.324-.782.003-1.267.436-1.493.966h-.023v-.798z" />
      </svg>
    );
  }

  if (["go"].includes(ext || "")) {
    return (
      <svg className="w-4 h-4 text-cyan-400" viewBox="0 0 24 24" fill="currentColor">
        <path d="M1.811 10.715c-.051 0-.089-.027-.089-.076l.116-.186c.021-.049.073-.074.124-.074h3.149c.052 0 .089.049.073.098l-.097.173c-.017.049-.068.086-.115.086l-3.161-.021zm-1.238.736c-.05 0-.086-.025-.086-.074l.114-.185c.022-.05.073-.075.125-.075h4.032c.05 0 .084.05.073.1l-.046.157c-.012.05-.063.087-.113.087l-4.099-.01zm1.999.738c-.05 0-.086-.026-.086-.075l.079-.173c.021-.049.063-.074.113-.074h1.765c.05 0 .087.039.087.087l-.009.149c0 .05-.05.087-.1.087l-1.849-.001z" />
        <path d="M11.371 10.168c-.644.173-1.085.3-1.72.473-.158.04-.17.052-.307-.102-.16-.178-.283-.296-.51-.4-.69-.316-1.36-.224-1.985.196-.743.5-1.126 1.228-1.116 2.131.01.894.624 1.619 1.513 1.726.764.093 1.403-.17 1.921-.74.107-.118.201-.25.309-.392h-2.123c-.219 0-.273-.136-.2-.313.134-.325.383-.872.528-1.145.032-.062.107-.163.256-.163h3.947c-.025.31-.025.618-.078.924-.154.89-.494 1.706-1.062 2.406-.928 1.145-2.099 1.79-3.574 1.92-1.218.107-2.326-.18-3.272-.945-1.068-.866-1.59-2.002-1.562-3.345.032-1.587.72-2.896 1.962-3.905 1.023-.83 2.224-1.22 3.556-1.187 1.071.026 2.025.36 2.854 1.016.332.263.61.577.855.923.071.1.031.163-.093.197z" />
      </svg>
    );
  }

  if (["md", "mdx"].includes(ext || "")) {
    return (
      <svg className="w-4 h-4 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
      </svg>
    );
  }

  if (["json", "yaml", "yml", "toml"].includes(ext || "")) {
    return (
      <svg className="w-4 h-4 text-yellow-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5" />
      </svg>
    );
  }

  // Default file icon
  return (
    <svg className="w-4 h-4 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
    </svg>
  );
}

function TreeNode({
  node,
  repoName,
  currentPath,
  depth = 0,
}: {
  node: FileNode;
  repoName: string;
  currentPath?: string;
  depth?: number;
}) {
  const [isExpanded, setIsExpanded] = useState(depth < 2);
  const isActive = currentPath === node.path;

  if (node.type === "directory") {
    return (
      <div>
        <button
          onClick={() => setIsExpanded(!isExpanded)}
          className={`w-full flex items-center gap-2 px-2 py-1.5 text-sm hover:bg-muted/50 rounded transition-colors ${
            isActive ? "bg-muted" : ""
          }`}
          style={{ paddingLeft: `${depth * 12 + 8}px` }}
        >
          <svg
            className={`w-3 h-3 text-muted-foreground transition-transform ${isExpanded ? "rotate-90" : ""}`}
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path fillRule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clipRule="evenodd" />
          </svg>
          <FileIcon type="directory" name={node.name} />
          <span className="truncate">{node.name}</span>
        </button>
        {isExpanded && node.children && (
          <div>
            {node.children.map((child) => (
              <TreeNode
                key={child.path}
                node={child}
                repoName={repoName}
                currentPath={currentPath}
                depth={depth + 1}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <Link
      href={`/repo/${repoName}/blob/${node.path}`}
      className={`flex items-center gap-2 px-2 py-1.5 text-sm hover:bg-muted/50 rounded transition-colors ${
        isActive ? "bg-muted text-foreground" : "text-muted-foreground"
      }`}
      style={{ paddingLeft: `${depth * 12 + 20}px` }}
    >
      <FileIcon type="file" name={node.name} />
      <span className="truncate">{node.name}</span>
    </Link>
  );
}

export function FileTree({ files, repoName, currentPath }: FileTreeProps) {
  return (
    <div className="py-2">
      {files.map((node) => (
        <TreeNode key={node.path} node={node} repoName={repoName} currentPath={currentPath} />
      ))}
    </div>
  );
}
