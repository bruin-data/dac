import { useState, useMemo } from "react";
import { Link } from "react-router-dom";
import type { DashboardListLayoutProps } from "../../types/template";

export function BruinDashboardListLayout({ dashboards, adminEnabled }: DashboardListLayoutProps) {
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    if (!query.trim()) return dashboards;
    const q = query.toLowerCase();
    return dashboards.filter(
      (d) =>
        d.name.toLowerCase().includes(q) ||
        d.description?.toLowerCase().includes(q) ||
        d.connection?.toLowerCase().includes(q),
    );
  }, [dashboards, query]);

  if (!dashboards.length) {
    return (
      <div className="max-w-[860px] mx-auto px-4 sm:px-6 pt-16 sm:pt-24 pb-8">
        <h1 className="text-lg font-semibold tracking-tight text-[var(--dac-text-primary)] mb-6">
          Dashboards
        </h1>
        <div className="border border-dashed border-[var(--dac-border)] rounded px-6 py-12 text-center">
          <p className="text-[13px] text-[var(--dac-text-secondary)] mb-1">No dashboards found</p>
          <p className="text-[12px] text-[var(--dac-text-muted)]">
            Create a <code className="font-mono text-[var(--dac-text-secondary)] bg-[var(--dac-surface)] px-1.5 py-0.5 rounded text-[11px]">.yml</code> file in your dashboards directory to get started.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-[860px] mx-auto px-4 sm:px-6 pt-16 sm:pt-24 pb-8">
      <div className="flex items-center justify-between mb-5 animate-in">
        <h1 className="text-lg font-semibold tracking-tight text-[var(--dac-text-primary)]">
          Dashboards
        </h1>
      </div>

      <div className="mb-4 animate-in" style={{ animationDelay: "20ms" }}>
        <div className="relative">
          <svg
            width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor"
            strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"
            className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--dac-text-muted)] pointer-events-none"
          >
            <circle cx="7" cy="7" r="4.5" />
            <path d="M10.5 10.5L14 14" />
          </svg>
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search dashboards..."
            className="w-full h-8 pl-8 pr-3 rounded text-[13px] border border-[var(--dac-border)] bg-[var(--dac-background)] text-[var(--dac-text-primary)] placeholder:text-[var(--dac-text-muted)] focus:outline-none focus:border-[var(--dac-accent)] transition-colors duration-100"
          />
        </div>
      </div>

      <div className="animate-in" style={{ animationDelay: "40ms" }}>
        <table className="w-full text-left border-collapse">
          <thead>
            <tr className="border-b border-[var(--dac-border)]">
              <th className="text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] pb-2 pl-3">Name</th>
              <th className="text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] pb-2 hidden sm:table-cell">Connection</th>
              <th className="text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] pb-2 text-right pr-3 hidden sm:table-cell">Widgets</th>
              <th className="text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] pb-2 text-right pr-3 hidden sm:table-cell">Filters</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-8 text-center text-[12px] text-[var(--dac-text-muted)]">
                  No dashboards matching "{query}"
                </td>
              </tr>
            ) : (
              filtered.map((d) => (
                <tr key={d.name} className="group border-b border-[var(--dac-border)] last:border-0">
                  <td className="py-0 pl-0">
                    <Link
                      to={`/d/${encodeURIComponent(d.name)}`}
                      className="block px-3 py-2.5 no-underline"
                    >
                      <span className="text-[13px] font-medium text-[var(--dac-text-primary)] group-hover:text-[var(--dac-accent)] transition-colors duration-100">
                        {d.name}
                      </span>
                      {d.description && (
                        <span className="block text-[12px] text-[var(--dac-text-muted)] truncate mt-0.5 max-w-[420px]">
                          {d.description}
                        </span>
                      )}
                    </Link>
                  </td>
                  <td className="py-2.5 hidden sm:table-cell">
                    <Link to={`/d/${encodeURIComponent(d.name)}`} className="no-underline">
                      {d.connection && (
                        <span className="text-[12px] text-[var(--dac-text-muted)] font-mono">
                          {d.connection}
                        </span>
                      )}
                    </Link>
                  </td>
                  <td className="py-2.5 text-right pr-3 hidden sm:table-cell">
                    <Link to={`/d/${encodeURIComponent(d.name)}`} className="no-underline">
                      <span className="text-[12px] text-[var(--dac-text-muted)] tabular-nums">
                        {d.widget_count ?? "—"}
                      </span>
                    </Link>
                  </td>
                  <td className="py-2.5 text-right pr-3 hidden sm:table-cell">
                    <Link to={`/d/${encodeURIComponent(d.name)}`} className="no-underline">
                      <span className="text-[12px] text-[var(--dac-text-muted)] tabular-nums">
                        {d.filter_count || "—"}
                      </span>
                    </Link>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {adminEnabled && (
        <div className="mt-8 text-center">
          <Link
            to="/admin"
            className="text-[11px] no-underline text-[var(--dac-text-muted)] hover:text-[var(--dac-text-secondary)]"
          >
            Admin
          </Link>
        </div>
      )}
    </div>
  );
}
