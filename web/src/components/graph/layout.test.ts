import { describe, expect, it } from "vitest";
import { docs, profile } from "../../test/fixtures";
import { buildGraph, fmtSpan, keyOf, roleActiveIn, yearDensity, ym } from "./layout";

describe("layout", () => {
  const { nodes, edges } = buildGraph(profile, docs, "Test Person");
  const kinds = (k: string) => nodes.filter((n) => n.kind === k);

  it("puts you in the centre with your roles, skills, industries and sources around", () => {
    expect(nodes[0]).toMatchObject({ id: "me", x: 0, y: 0 });
    expect(kinds("role")).toHaveLength(2);
    expect(kinds("skill").map((n) => n.label)).toEqual(["Go", "PostgreSQL", "Rust"]);
    expect(kinds("industry").map((n) => n.label)).toEqual(["Payments"]);
    expect(kinds("source")).toHaveLength(1);
    expect(kinds("leaf")).toHaveLength(1);
  });

  it("links skills to the roles that used them, and unused skills to you", () => {
    const go = kinds("skill").find((n) => n.label === "Go")!;
    const rust = kinds("skill").find((n) => n.label === "Rust")!;
    expect(edges.filter((e) => e.b === go.id && e.kind === "skill")).toHaveLength(2);
    expect(edges.find((e) => e.b === rust.id)).toMatchObject({ a: "me", kind: "loose" });
  });

  it("matches skill names loosely", () => {
    expect(keyOf("Node.js")).toBe(keyOf("node"));
    expect(keyOf("Golang")).toBe("go");
  });

  it("reads dates and spans", () => {
    expect(ym("2020-03")).toBe(2020 * 12 + 2);
    expect(ym("2020", true)).toBe(2020 * 12 + 11);
    expect(ym("soon")).toBeNull();
    expect(fmtSpan(profile.experience![1])).toMatch(/present$/);
    expect(roleActiveIn(profile.experience![0], 2020)).toBe(true);
    expect(roleActiveIn(profile.experience![0], 2023)).toBe(false);
  });

  it("measures how much work each year held", () => {
    const d = yearDensity(profile.experience!);
    expect(d[0].year).toBe(2019);
    expect(d.find((y) => y.year === 2020)!.roles).toBe(1);
    expect(d.at(-1)!.year).toBe(new Date().getFullYear());
  });
});
