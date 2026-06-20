/// <reference types="vite/client" />

import type { StaticPayload } from "./api/client";

declare global {
  interface Window {
    __DAC_STATIC__?: StaticPayload;
  }
}

export {};
