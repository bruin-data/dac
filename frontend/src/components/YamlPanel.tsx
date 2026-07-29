import { useState, useEffect } from "react";
import { getDashboardRaw } from "../api/client";
import { useShikiHighlight } from "../hooks/useShikiHighlight";
import { ResizeHandle } from "./ResizeHandle";

interface YamlPanelProps {
  dashboardName: string;
  fileType?: "yaml" | "tsx";
  isOpen: boolean;
  onClose: () => void;
  onResize: (delta: number) => void;
  onResizeStart: () => void;
  onResizeEnd: () => void;
}

function CopyIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

export function YamlPanel({ dashboardName, fileType, isOpen, onClose, onResize, onResizeStart, onResizeEnd }: YamlPanelProps) {
  const [yaml, setYaml] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const lang = fileType === "tsx" ? "tsx" : "yaml";
  const html = useShikiHighlight(yaml, lang);

  useEffect(() => {
    if (!isOpen || !dashboardName) return;
    setError(null);
    getDashboardRaw(dashboardName)
      .then(setYaml)
      .catch((err) => setError(err.message));
  }, [isOpen, dashboardName]);

  const handleCopy = () => {
    if (yaml === null) return;
    navigator.clipboard.writeText(yaml).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <div
      className={`yaml-sidebar ${isOpen ? "" : "yaml-sidebar-closed"}`}
    >
      {isOpen && <ResizeHandle side="left" onResize={onResize} onResizeStart={onResizeStart} onResizeEnd={onResizeEnd} />}
      <button
        onClick={handleCopy}
        disabled={yaml === null}
        title={copied ? "Copied" : "Copy"}
        className="absolute top-2.5 right-10 z-10 w-6 h-6 flex items-center justify-center rounded hover:bg-[var(--dac-surface-hover)] text-[var(--dac-text-muted)] hover:text-[var(--dac-text-secondary)] transition-colors disabled:opacity-40 disabled:cursor-default"
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </button>
      <button
        onClick={onClose}
        className="absolute top-2.5 right-2.5 z-10 w-6 h-6 flex items-center justify-center rounded hover:bg-[var(--dac-surface-hover)] text-[var(--dac-text-muted)] hover:text-[var(--dac-text-secondary)] transition-colors"
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
          <path d="M4 4L12 12M12 4L4 12" />
        </svg>
      </button>

      <div className="yaml-highlight flex-1 overflow-auto p-4 pt-10">
        {error ? (
          <div className="text-[12px] text-[var(--dac-error)]">{error}</div>
        ) : yaml === null ? (
          <div className="space-y-2">
            <div className="skeleton h-3 w-full" />
            <div className="skeleton h-3 w-3/4" />
            <div className="skeleton h-3 w-5/6" />
          </div>
        ) : html ? (
          <div dangerouslySetInnerHTML={{ __html: html }} />
        ) : (
          <pre className="text-[12px] leading-[1.6] font-mono text-[var(--dac-text-secondary)] m-0">
            {yaml}
          </pre>
        )}
      </div>
    </div>
  );
}
