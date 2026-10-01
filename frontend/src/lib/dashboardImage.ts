const MAX_IMAGE_URL_LENGTH = 4096;

/**
 * Dashboard image URLs are untrusted query data. Only absolute HTTPS URLs on
 * another origin are rendered; relative or same-origin URLs could otherwise
 * call the DAC server with the viewer's credentials, and active URL schemes
 * can execute attacker content.
 */
export function dashboardImageSrc(value: unknown): string {
  if (typeof value !== "string") return "";

  const candidate = value.trim();
  if (!candidate || candidate.length > MAX_IMAGE_URL_LENGTH) return "";
  // new URL() accepts forms like "https:/path" and "https:path"; require the
  // literal prefix so only unambiguous absolute URLs pass.
  if (!/^https:\/\/[^/\\]/i.test(candidate)) return "";

  try {
    const parsed = new URL(candidate);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password) return "";
    if (parsed.origin === window.location.origin) return "";
    return parsed.href;
  } catch {
    return "";
  }
}
