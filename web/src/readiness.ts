// Same formula as internal/readiness (Go); the server-side tests pin the numbers.
import { ASPECTS, type AspectKey, type Journey, type JourneyNode, type Progress } from "./types";

const clamp01 = (x: number) => (Number.isFinite(x) ? Math.min(1, Math.max(0, x)) : 0);
const hasTest = (n: JourneyNode) => n.test.kind !== "none";

export const units = (n: JourneyNode) => n.tasks.length + (hasTest(n) ? 1 : 0);
export const doneUnits = (n: JourneyNode, p: Progress) =>
  n.tasks.filter((_, i) => p[`${n.id}:${i}`]).length + (hasTest(n) && p[`${n.id}:test`] ? 1 : 0);

export function nodeScore(n: JourneyNode, p: Progress) {
  const b = clamp01(n.baseline);
  const u = units(n);
  return u ? b + (1 - b) * (doneUnits(n, p) / u) : b;
}
export const isDone = (n: JourneyNode, p: Progress) => units(n) > 0 && doneUnits(n, p) === units(n);

// Placeholders (nodes still being written, or that failed) don't count until they arrive.
export const unwritten = (j: Journey) => new Set([...(j.pending ?? []), ...(j.failed ?? [])]);
const writtenNodes = (j: Journey) => {
  const u = unwritten(j);
  return u.size ? j.nodes.filter((n) => !u.has(n.id)) : j.nodes;
};

export function aspectScore(j: Journey, k: AspectKey, p: Progress) {
  const ns = writtenNodes(j).filter((n) => n.aspect === k);
  return ns.length ? ns.reduce((s, n) => s + nodeScore(n, p), 0) / ns.length : 0;
}

export function overallScore(j: Journey, p: Progress) {
  const present = ASPECTS.filter((a) => writtenNodes(j).some((n) => n.aspect === a.key));
  const w = present.reduce((s, a) => s + a.weight, 0) || 1;
  return present.reduce((s, a) => s + a.weight * aspectScore(j, a.key, p), 0) / w;
}

export const nextIn = (j: Journey, k: AspectKey, p: Progress) => writtenNodes(j).find((n) => n.aspect === k && !isDone(n, p));

export function bestNext(j: Journey, p: Progress) {
  const ranked = [...ASPECTS].sort((a, b) => aspectScore(j, a.key, p) - aspectScore(j, b.key, p));
  for (const a of ranked) {
    const n = nextIn(j, a.key, p);
    if (n) return n;
  }
  return null;
}

export const pct = (x: number) => Math.round(clamp01(x) * 100);
