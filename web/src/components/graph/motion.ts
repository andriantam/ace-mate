import type { GNode, NodeKind } from "./layout";

// The motion that makes the profile map feel alive. Pure functions, so they can be tested:
// the graph component calls them every animation frame and moves the SVG elements.
//
//  - Idle drift: every node floats on its own slow orbit.
//  - Depth: rings further out sit "closer to the viewer". As the pointer moves away from
//    the centre they shift more than the inner rings (parallax), and the whole map tilts a
//    little, which reads as 3D.
//  - Focus: the node nearest the pointer grows and leans toward it; nodes around it are
//    pushed away to give it room.

export type Body = {
  id: string;
  kind: NodeKind;
  x: number;
  y: number;
  depth: number; // 0 (the centre) to 1 (the outer ring)
  amp: number; // idle drift radius, in map units
  phase: number; // where on its orbit it starts
  speed: number; // orbit speed, radians per second
};

export type Offset = { dx: number; dy: number; s: number };
export type Point = { x: number; y: number };

const DEPTH: Record<NodeKind, number> = { me: 0, source: 0.15, role: 0.35, leaf: 0.45, skill: 0.7, industry: 1 };
const AMP: Record<NodeKind, number> = { me: 1.5, source: 4, role: 5, leaf: 3, skill: 7, industry: 8 };

/** A stable number in [0, 1) from a string, so each node keeps its own orbit across renders. */
export function hash01(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  return (h >>> 0) / 2 ** 32;
}

export function toBody(n: GNode): Body {
  const h = hash01(n.id);
  return { id: n.id, kind: n.kind, x: n.x, y: n.y, depth: DEPTH[n.kind], amp: AMP[n.kind], phase: h * Math.PI * 2, speed: 0.35 + 0.3 * hash01(n.id + "~") };
}

/** The node nearest to p within reach (map units), or null. */
export function nearest(bodies: Body[], p: Point | null, reach: number): string | null {
  if (!p) return null;
  let best: string | null = null;
  let bestD = reach;
  for (const b of bodies) {
    const d = Math.hypot(b.x - p.x, b.y - p.y);
    if (d < bestD) {
      best = b.id;
      bestD = d;
    }
  }
  return best;
}

export const PUSH_RADIUS = 150;
const PUSH = 36;
const PARALLAX = 26;

/**
 * Where a node should be at time t (seconds), relative to its place in the layout.
 * pointer is in map units, or null when the pointer is outside the map. tilt is the
 * pointer's direction from the centre, each axis in [-1, 1].
 */
export function offsetFor(b: Body, t: number, pointer: Point | null, tilt: Point, focus: string | null): Offset {
  // Idle drift: a slow ellipse, different for every node.
  let dx = Math.cos(t * b.speed + b.phase) * b.amp;
  let dy = Math.sin(t * b.speed * 0.8 + b.phase * 1.3) * b.amp * 0.8;

  // Parallax: outer rings move against the pointer more than inner ones.
  dx -= tilt.x * b.depth * PARALLAX;
  dy -= tilt.y * b.depth * PARALLAX;

  let s = 1;
  if (pointer) {
    const vx = b.x - pointer.x;
    const vy = b.y - pointer.y;
    const d = Math.hypot(vx, vy) || 1;
    if (b.id === focus) {
      // The focused node leans a quarter of the way toward the pointer and grows.
      dx -= vx * 0.25;
      dy -= vy * 0.25;
      s = b.kind === "me" ? 1.08 : 1.45;
    } else if (d < PUSH_RADIUS) {
      // Neighbours are pushed away, more the closer they are. The centre barely moves.
      const f = PUSH * (1 - d / PUSH_RADIUS) ** 2 * (b.kind === "me" ? 0.2 : 1);
      dx += (vx / d) * f;
      dy += (vy / d) * f;
    }
  }
  return { dx, dy, s };
}

/** Moves current a step toward target, for smooth, springy motion. */
export function approach(current: Offset, target: Offset, k: number): Offset {
  return {
    dx: current.dx + (target.dx - current.dx) * k,
    dy: current.dy + (target.dy - current.dy) * k,
    s: current.s + (target.s - current.s) * k,
  };
}

/** A slow wander for the tilt while nobody is pointing at the map. */
export function idleTilt(t: number): Point {
  return { x: Math.sin(t * 0.13) * 0.22, y: Math.cos(t * 0.11) * 0.18 };
}
