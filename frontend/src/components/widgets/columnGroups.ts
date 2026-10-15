import type { TableColumn } from "../../types/dashboard";

interface TableColumnGroup {
  id: string;
  name: string;
  columns: string[];
  summary: string;
  collapsed: boolean;
}

export function buildColumnGroups(columns: TableColumn[]): TableColumnGroup[] {
  const groups: TableColumnGroup[] = [];
  let group: TableColumnGroup | undefined;
  for (const column of columns) {
    if (!column.group) {
      group = undefined;
      continue;
    }
    if (group?.name !== column.group) {
      group = {
        id: JSON.stringify([column.group, column.name]),
        name: column.group,
        columns: [],
        summary: column.name,
        collapsed: column.collapsed ?? false,
      };
      groups.push(group);
    }
    group.columns.push(column.name);
  }
  // A single member has no details to toggle; render it as an ordinary column.
  return groups.filter((group) => group.columns.length > 1);
}

export type ColumnGroupState = Record<string, boolean>;

export function isColumnGroupCollapsed(group: TableColumnGroup, state: ColumnGroupState): boolean {
  const stored = Object.prototype.hasOwnProperty.call(state, group.id) ? state[group.id] : undefined;
  return stored ?? group.collapsed ?? false;
}

export function readColumnGroupState(key: string): ColumnGroupState {
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(key) ?? "{}");
    if (!value || typeof value !== "object" || Array.isArray(value)) return {};
    return Object.fromEntries(Object.entries(value).filter(([, collapsed]) => typeof collapsed === "boolean"));
  } catch {
    return {};
  }
}

export function collapsedColumnNames(groups: TableColumnGroup[], state: ColumnGroupState): Set<string> {
  return new Set(groups.flatMap((group) =>
    isColumnGroupCollapsed(group, state)
      ? group.columns.filter((name) => name !== group.summary)
      : [],
  ));
}
