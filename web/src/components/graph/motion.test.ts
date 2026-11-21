import { describe, expect, it } from "vitest";
import { approach, hash01, idleTilt, nearest, offsetFor, PUSH_RADIUS, toBody, type Body } from "./motion";

const body = (over: Partial<Body> = {}): Body => ({ id: "n", kind: "skill", x: 100, y: 0, depth: 0.7, amp: 0, phase: 0, speed: 0, ...over });
const still = { x: 0, y: 0 };

describe("motion", () => {
  it("gives every node a stable orbit", () => {
    expect(hash01("role-1")).toBe(hash01("role-1"));
    expect(hash01("role-1")).not.toBe(hash01("role-2"));
    const n = { id: "skill-0", kind: "skill" as const, label: "Go", x: 10, y: 20, r: 5, angle: 0 };
    expect(toBody(n)).toEqual(toBody(n));
  });

  it("drifts while idle, within its radius", () => {
    const b = body({ amp: 7, speed: 0.5 });
    const seen = [0, 1, 2, 3].map((t) => offsetFor(b, t, null, still, null));
    expect(new Set(seen.map((o) => o.dx.toFixed(3))).size).toBeGreaterThan(1);
    for (const o of seen) expect(Math.hypot(o.dx, o.dy)).toBeLessThanOrEqual(7 + 1e-9);
  });

  it("pushes neighbours away from the pointer, more when closer", () => {
    const near = offsetFor(body({ x: 40 }), 0, { x: 0, y: 0 }, still, "other");
    const far = offsetFor(body({ x: 120 }), 0, { x: 0, y: 0 }, still, "other");
    const out = offsetFor(body({ x: PUSH_RADIUS + 10 }), 0, { x: 0, y: 0 }, still, "other");
    expect(near.dx).toBeGreaterThan(far.dx);
    expect(far.dx).toBeGreaterThan(0);
    expect(out.dx).toBe(0);
  });

  it("grows the focused node and leans it toward the pointer", () => {
    const o = offsetFor(body({ id: "n", x: 40 }), 0, { x: 0, y: 0 }, still, "n");
    expect(o.s).toBeGreaterThan(1);
    expect(o.dx).toBeLessThan(0);
  });

  it("moves outer rings more than inner ones (parallax)", () => {
    const tilt = { x: 1, y: 0 };
    const inner = offsetFor(body({ depth: 0.15 }), 0, null, tilt, null);
    const outer = offsetFor(body({ depth: 1 }), 0, null, tilt, null);
    expect(Math.abs(outer.dx)).toBeGreaterThan(Math.abs(inner.dx));
    expect(offsetFor(body({ kind: "me", depth: 0 }), 0, null, tilt, null).dx).toBe(0);
  });

  it("finds the nearest node within reach", () => {
    const bodies = [body({ id: "a", x: 0, y: 0 }), body({ id: "b", x: 30, y: 0 })];
    expect(nearest(bodies, { x: 25, y: 0 }, 40)).toBe("b");
    expect(nearest(bodies, { x: 300, y: 0 }, 40)).toBeNull();
    expect(nearest(bodies, null, 40)).toBeNull();
  });

  it("eases toward the target and wanders while idle", () => {
    const o = approach({ dx: 0, dy: 0, s: 1 }, { dx: 10, dy: -10, s: 2 }, 0.5);
    expect(o).toEqual({ dx: 5, dy: -5, s: 1.5 });
    expect(Math.abs(idleTilt(3).x)).toBeLessThan(0.25);
  });
});
