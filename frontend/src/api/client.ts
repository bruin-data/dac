import type { DashboardSummary, Dashboard, BatchDataResponse, WidgetData } from "../types/dashboard";
import type { Theme } from "../types/theme";

const BASE = "/api/v1";

// --- Static payload detection ---

export interface StaticPayload {
  config: ServerConfig;
  dashboard: Dashboard;
  dashboards: DashboardSummary[];
  widgetData: Record<string, WidgetData>;
  filters: Record<string, unknown>;
}

export function getStaticPayload(): StaticPayload | undefined {
  return window.__DAC_STATIC__;
}

// --- API functions ---

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`API error ${res.status}: ${body}`);
  }
  return res.json();
}

export interface ServerConfig {
  template: string;
  tokens?: Record<string, string>;
  admin_enabled?: boolean;
}

export async function fetchConfig(): Promise<ServerConfig> {
  const sp = getStaticPayload();
  if (sp) return sp.config;
  return fetchJSON<ServerConfig>(`${BASE}/config`);
}

export async function listDashboards(): Promise<DashboardSummary[]> {
  const sp = getStaticPayload();
  if (sp) return sp.dashboards;
  const data = await fetchJSON<{ dashboards: DashboardSummary[] }>(`${BASE}/dashboards`);
  return data.dashboards;
}

export async function getDashboard(name: string): Promise<Dashboard> {
  const sp = getStaticPayload();
  if (sp) return sp.dashboard;
  return fetchJSON<Dashboard>(`${BASE}/dashboards/${encodeURIComponent(name)}`);
}

export async function getDashboardRaw(name: string): Promise<string> {
  const res = await fetch(`${BASE}/dashboards/${encodeURIComponent(name)}/raw`);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`API error ${res.status}: ${body}`);
  }
  return res.text();
}

export async function fetchDashboardData(
  name: string,
  filters?: Record<string, unknown>,
): Promise<Record<string, WidgetData>> {
  const sp = getStaticPayload();
  if (sp) return sp.widgetData;
  const data = await fetchJSON<BatchDataResponse>(
    `${BASE}/dashboards/${encodeURIComponent(name)}/data`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ filters: filters ?? {} }),
    },
  );
  return data.widgets;
}

/**
 * Stream widget data via NDJSON. Calls onWidget for each widget result
 * as it arrives from the server. Returns an abort function.
 */
export function streamDashboardData(
  name: string,
  filters: Record<string, unknown> | undefined,
  onWidget: (id: string, data: WidgetData) => void,
  onDone: () => void,
  onError: (err: Error) => void,
): () => void {
  // In static mode, synchronously emit baked data.
  const sp = getStaticPayload();
  if (sp) {
    for (const [id, data] of Object.entries(sp.widgetData)) {
      onWidget(id, data);
    }
    onDone();
    return () => {};
  }

  const controller = new AbortController();

  (async () => {
    try {
      const res = await fetch(
        `${BASE}/dashboards/${encodeURIComponent(name)}/stream`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ filters: filters ?? {} }),
          signal: controller.signal,
        },
      );

      if (!res.ok) {
        const body = await res.text();
        throw new Error(`API error ${res.status}: ${body}`);
      }

      const reader = res.body?.getReader();
      if (!reader) throw new Error("No response body");

      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // Process complete lines.
        let newlineIdx;
        while ((newlineIdx = buffer.indexOf("\n")) !== -1) {
          const line = buffer.slice(0, newlineIdx).trim();
          buffer = buffer.slice(newlineIdx + 1);

          if (!line) continue;

          try {
            const msg = JSON.parse(line) as { id: string; data: WidgetData };
            onWidget(msg.id, msg.data);
          } catch {
            // Skip malformed lines.
          }
        }
      }

      onDone();
    } catch (err) {
      if ((err as DOMException)?.name === "AbortError") return;
      onError(err instanceof Error ? err : new Error(String(err)));
    }
  })();

  return () => controller.abort();
}

export async function fetchWidgetData(
  dashboardName: string,
  widgetId: string,
  filters?: Record<string, unknown>,
): Promise<WidgetData> {
  const sp = getStaticPayload();
  if (sp) return sp.widgetData[widgetId];
  return fetchJSON<WidgetData>(
    `${BASE}/dashboards/${encodeURIComponent(dashboardName)}/widgets/${encodeURIComponent(widgetId)}/query`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ filters: filters ?? {} }),
    },
  );
}

export async function listThemes(): Promise<string[]> {
  const data = await fetchJSON<{ themes: string[] }>(`${BASE}/themes`);
  return data.themes;
}

export async function getTheme(name: string): Promise<Theme> {
  return fetchJSON<Theme>(`${BASE}/themes/${encodeURIComponent(name)}`);
}

// --- Admin API ---

const ADMIN_PASSWORD_KEY = "dac_admin_password";

export function getAdminPassword(): string | null {
  return sessionStorage.getItem(ADMIN_PASSWORD_KEY);
}

export function setAdminPassword(password: string): void {
  sessionStorage.setItem(ADMIN_PASSWORD_KEY, password);
}

export function clearAdminPassword(): void {
  sessionStorage.removeItem(ADMIN_PASSWORD_KEY);
}

function adminHeaders(): Record<string, string> {
  const password = getAdminPassword();
  if (!password) {
    throw new Error("Not authenticated");
  }
  return { Authorization: `Bearer ${password}` };
}

export async function adminLogin(password: string): Promise<void> {
  const res = await fetch(`${BASE}/admin/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
  if (!res.ok) {
    throw new Error("Invalid password");
  }
  setAdminPassword(password);
}

export interface AdminConnections {
  connections: Record<string, Array<{ name: string; [key: string]: unknown }>>;
}

export async function listConnections(): Promise<AdminConnections> {
  return fetchJSON<AdminConnections>(`${BASE}/admin/connections`, {
    headers: adminHeaders(),
  });
}

export async function createConnection(
  type: string,
  name: string,
  fields: Record<string, unknown>,
): Promise<void> {
  await fetchJSON(`${BASE}/admin/connections`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...adminHeaders() },
    body: JSON.stringify({ type, name, fields }),
  });
}

export async function updateConnection(
  type: string,
  name: string,
  fields: Record<string, unknown>,
): Promise<void> {
  await fetchJSON(
    `${BASE}/admin/connections/${encodeURIComponent(type)}/${encodeURIComponent(name)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...adminHeaders() },
      body: JSON.stringify({ fields }),
    },
  );
}

export async function deleteConnection(type: string, name: string): Promise<void> {
  await fetchJSON(
    `${BASE}/admin/connections/${encodeURIComponent(type)}/${encodeURIComponent(name)}`,
    {
      method: "DELETE",
      headers: adminHeaders(),
    },
  );
}

export async function testConnection(
  type: string,
  name: string,
): Promise<{ ok: boolean }> {
  return fetchJSON<{ ok: boolean }>(
    `${BASE}/admin/connections/${encodeURIComponent(type)}/${encodeURIComponent(name)}/test`,
    {
      method: "POST",
      headers: adminHeaders(),
    },
  );
}
