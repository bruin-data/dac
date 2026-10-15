import { afterEach, describe, expect, it, vi } from "vitest";
import type { TableColumn } from "../../types/dashboard";
import { buildColumnGroups, collapsedColumnNames, readColumnGroupState } from "./columnGroups";

const columns: TableColumn[] = [
  { name: "region" },
  { name: "profit", group: "financials", collapsed: true },
  { name: "revenue", group: "financials" },
  { name: "cost", group: "orders" },
  { name: "shipping_cost", group: "orders" },
  { name: "orders", group: "financials" },
  { name: "average_order", group: "financials" },
  { name: "margin", group: "solo" },
  { name: "status" },
];

describe("buildColumnGroups", () => {
  it("splits separated blocks with the same name into independent groups", () => {
    const groups = buildColumnGroups(columns);
    expect(groups.map((group) => group.columns)).toEqual([
      ["profit", "revenue"],
      ["cost", "shipping_cost"],
      ["orders", "average_order"],
    ]);
    expect(new Set(groups.map((group) => group.id)).size).toBe(3);
  });

  it("takes the summary column and default state from the first member", () => {
    const [financials, orders] = buildColumnGroups(columns);
    expect(financials).toMatchObject({ summary: "profit", collapsed: true });
    expect(orders).toMatchObject({ summary: "cost", collapsed: false });
  });

  it("ends a block at an ungrouped column", () => {
    const groups = buildColumnGroups([
      { name: "a", group: "g" },
      { name: "b" },
      { name: "c", group: "g" },
    ]);
    expect(groups).toEqual([]);
  });

  it("drops single-column blocks", () => {
    expect(buildColumnGroups(columns).some((group) => group.name === "solo")).toBe(false);
  });
});

describe("collapsedColumnNames", () => {
  const groups = buildColumnGroups(columns);

  it("hides every member except the summary of collapsed groups", () => {
    expect([...collapsedColumnNames(groups, {})]).toEqual(["revenue"]);
  });

  it("lets stored state override the defaults", () => {
    const [financials, orders] = groups;
    const hidden = collapsedColumnNames(groups, { [financials.id]: false, [orders.id]: true });
    expect([...hidden]).toEqual(["shipping_cost"]);
  });
});

describe("readColumnGroupState", () => {
  afterEach(() => vi.unstubAllGlobals());

  function stubStorage(value: string | null) {
    vi.stubGlobal("window", { localStorage: { getItem: () => value } });
  }

  it("keeps only boolean entries", () => {
    stubStorage(JSON.stringify({ a: true, b: false, c: "yes", d: 1 }));
    expect(readColumnGroupState("key")).toEqual({ a: true, b: false });
  });

  it.each([
    ["missing", null],
    ["malformed JSON", "{"],
    ["an array", "[true]"],
    ["a primitive", "true"],
  ])("returns empty state for %s storage", (_, value) => {
    stubStorage(value);
    expect(readColumnGroupState("key")).toEqual({});
  });

  it("returns empty state when storage throws", () => {
    vi.stubGlobal("window", { localStorage: { getItem: () => { throw new Error("denied"); } } });
    expect(readColumnGroupState("key")).toEqual({});
  });
});
