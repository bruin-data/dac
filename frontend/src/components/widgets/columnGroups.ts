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
  const toggled = Object.prototype.hasOwnProperty.call(state, group.id) ? state[group.id] : undefined;
  return toggled ?? group.collapsed ?? false;
}

export function collapsedColumnNames(groups: TableColumnGroup[], state: ColumnGroupState): Set<string> {
  return new Set(groups.flatMap((group) =>
    isColumnGroupCollapsed(group, state)
      ? group.columns.filter((name) => name !== group.summary)
      : [],
  ));
}
