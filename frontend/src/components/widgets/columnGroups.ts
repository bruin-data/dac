import type { TableColumn } from "../../types/dashboard";

interface TableColumnGroup {
  id: string;
  name: string;
  columns: string[];
  summary: string;
  collapsed: boolean;
  hidden: boolean;
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
        hidden: column.hidden ?? false,
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
      ? group.columns
      : [],
  ));
}

// The outline shares the table's real columns. Hidden groups attach to a visible
// cell boundary instead of adding a placeholder column to the table.
export function buildColumnGroupOutline(
  columns: { name: string }[],
  groups: TableColumnGroup[],
  state: ColumnGroupState,
) {
  const byMember = new Map(groups.flatMap((group) => group.columns.map((name) => [name, group] as const)));
  const cells: {
    key: string;
    index: number;
    span: number;
    group?: TableColumnGroup;
    before: TableColumnGroup[];
    after: TableColumnGroup[];
  }[] = [];
  let pending: TableColumnGroup[] = [];
  let index = 0;
  for (const column of columns) {
    const group = byMember.get(column.name);
    if (group && column.name !== group.summary) continue;
    if (group && isColumnGroupCollapsed(group, state)) {
      pending.push(group);
      continue;
    }
    const span = group?.columns.length ?? 1;
    const before = cells.length ? [] : pending;
    if (cells.length) cells[cells.length - 1].after = pending;
    cells.push({ key: group?.id ?? column.name, index, span, group, before, after: [] });
    index += span;
    pending = [];
  }
  if (cells.length) cells[cells.length - 1].after = pending;
  return { cells, hidden: cells.length ? [] : pending, height: 26 };
}
