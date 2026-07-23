/// <reference types="vite/client" />

import type { StaticPayload } from "./api/client";

declare global {
  interface Window {
    __DAC_STATIC__?: StaticPayload;
    // Injected by the Bruin VS Code embed: dac-serve URL + initial dashboard.
    __DAC_API_BASE__?: string;
    __DAC_INITIAL_DASHBOARD__?: string;
  }
}

export {};
