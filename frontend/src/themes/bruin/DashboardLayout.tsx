import { Link } from "react-router-dom";
import type { DashboardLayoutProps } from "../../types/template";

const isStaticMode = window.__DAC_STATIC__ !== undefined;

export function BruinDashboardLayout({ dashboard, filterBar, headerActions, children }: DashboardLayoutProps) {
  return (
    <div className="max-w-[1400px] mx-auto px-4 sm:px-6 py-6 sm:py-8">
      <header className="mb-6 animate-in">
        <div className="flex items-start justify-between">
          <div>
            {!isStaticMode && (
              <Link
                to="/"
                className="inline-flex items-center gap-1 text-[12px] text-[var(--dac-text-muted)] hover:text-[var(--dac-text-secondary)] transition-colors duration-100 mb-3"
              >
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M10 12L6 8L10 4" />
                </svg>
                Dashboards
              </Link>
            )}
            <h1 className="text-xl font-semibold tracking-tight text-[var(--dac-text-primary)]">
              {dashboard.name}
            </h1>
            {dashboard.description && (
              <p className="text-[13px] text-[var(--dac-text-secondary)] mt-1 max-w-lg">
                {dashboard.description}
              </p>
            )}
          </div>
          {headerActions}
        </div>
      </header>

      {filterBar && (
        <div className="animate-in" style={{ animationDelay: "30ms" }}>
          {filterBar}
        </div>
      )}

      <div className="flex flex-col gap-4">
        {children}
      </div>
    </div>
  );
}
