import { useState, useEffect } from "react";
import type { HighlighterCore } from "shiki/core";

const THEME = "github-light";

// Lazily load only the languages we actually highlight (sql, yaml, tsx) instead
// of the full shiki bundle, which would emit a lazy chunk for every bundled
// grammar. The dynamic import keeps shiki out of the main bundle.
let highlighterPromise: Promise<HighlighterCore> | null = null;
function getHighlighter(): Promise<HighlighterCore> {
  highlighterPromise ??= (async () => {
    const [{ createHighlighterCore }, { createJavaScriptRegexEngine }, sql, yaml, tsx, githubLight] =
      await Promise.all([
        import("shiki/core"),
        import("shiki/engine/javascript"),
        import("@shikijs/langs/sql"),
        import("@shikijs/langs/yaml"),
        import("@shikijs/langs/tsx"),
        import("@shikijs/themes/github-light"),
      ]);
    return createHighlighterCore({
      themes: [githubLight.default],
      langs: [sql.default, yaml.default, tsx.default],
      engine: createJavaScriptRegexEngine(),
    });
  })();
  return highlighterPromise;
}

export function useShikiHighlight(code: string | null, lang: string): string | null {
  const [html, setHtml] = useState<string | null>(null);

  useEffect(() => {
    if (!code) {
      setHtml(null);
      return;
    }
    let cancelled = false;
    getHighlighter()
      .then((hl) => hl.codeToHtml(code, { lang, theme: THEME }))
      .then((result) => {
        if (!cancelled) setHtml(result);
      });
    return () => { cancelled = true; };
  }, [code, lang]);

  return html;
}
