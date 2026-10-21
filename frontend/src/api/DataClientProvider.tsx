import type { ReactNode } from "react";
import { DataClientContext, type DataClient } from "./dataClient";

export function DataClientProvider({
  client,
  children,
}: {
  client: DataClient;
  children: ReactNode;
}) {
  return <DataClientContext.Provider value={client}>{children}</DataClientContext.Provider>;
}
