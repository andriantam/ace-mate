import type { DocumentRow, Profile, Role, Stack } from "../../types";

// Where everything sits on the profile map: you in the centre, your roles on the first ring,
// the skills each role used and the industries you worked in further out, the documents
// everything came from close to the centre, and highlights as small leaves around each role.

export type NodeKind = "me" | "role" | "skill" | "industry" | "source" | "leaf";
export type GNode = {
  id: string;
  kind: NodeKind;
  label: string;
  sub?: string;
  x: number;
  y: number;
  r: number;
  angle: number;
  role?: Role;
  stack?: Stack;
  doc?: DocumentRow;
  text?: string;
};
export type Edge = { a: string; b: string; kind: "spine" | "skill" | "industry" | "leaf" | "source" | "evidence" | "loose" };

export const R = { source: 92, role: 205, skill: 335, industry: 455 };
const TAU = Math.PI * 2;

export const keyOf = (s: string) => s.toLowerCase().replace(/[^a-z0-9+#]/g, "").replace(/js$/, "").replace(/^golang$/, "go");
const related = (a: string, b: string) => {
  const x = keyOf(a), y = keyOf(b);
  if (!x || !y) return false;
  if (x === y) return true;
  // "React / Angular" and "React" are related; "Java" and "JavaScript" are not.
  const words = (s: string) => s.toLowerCase().split(/[^a-z0-9+#.]+/).map(keyOf).filter(Boolean);
  const wa = words(a), wb = words(b);
  return wa.some((w) => wb.includes(w) && w.length > 1);
};

// ── dates ──

export function ym(s: string, end = false): number | null {
  const m = /^(\d{4})(?:-(\d{2}))?$/.exec(s);
  if (!m) return null;
  return Number(m[1]) * 12 + (m[2] ? Number(m[2]) - 1 : end ? 11 : 0);
}
const nowYM = () => new Date().getFullYear() * 12 + new Date().getMonth();
export function span(r: Role): [number, number] | null {
  const a = ym(r.start);
  if (a === null) return null;
  const b = r.end ? ym(r.end, true) : nowYM();
  return [a, Math.max(a, b ?? a)];
}
export const roleActiveIn = (r: Role, year: number) => {
  const s = span(r);
  return s !== null && s[0] <= year * 12 + 11 && s[1] >= year * 12;
};
export function fmtSpan(r: Role) {
  const f = (s: string) => {
    const m = /^(\d{4})(?:-(\d{2}))?$/.exec(s);
    if (!m) return "";
    return m[2] ? new Date(Number(m[1]), Number(m[2]) - 1).toLocaleString(undefined, { month: "short", year: "numeric" }) : m[1];
  };
  const a = f(r.start);
  if (!a) return "";
  return `${a} – ${r.end ? f(r.end) : "present"}`;
}

// ── layout ──

const circMean = (angles: number[]) => Math.atan2(angles.reduce((s, a) => s + Math.sin(a), 0), angles.reduce((s, a) => s + Math.cos(a), 0));
const norm = (a: number) => ((a % TAU) + TAU) % TAU;

/** Spreads items around a ring near their wanted angles, keeping a minimum gap. */
function spread<T extends { want: number }>(items: T[], minGap: number): (T & { angle: number })[] {
  if (!items.length) return [];
  const gap = Math.min(minGap, TAU / items.length);
  const sorted = [...items].map((it) => ({ ...it, angle: norm(it.want) })).sort((a, b) => a.angle - b.angle);
  for (let pass = 0; pass < 4; pass++) {
    for (let i = 1; i < sorted.length; i++) {
      if (sorted[i].angle - sorted[i - 1].angle < gap) sorted[i].angle = sorted[i - 1].angle + gap;
    }
    const over = sorted[sorted.length - 1].angle - (sorted[0].angle + TAU - gap);
    if (over > 0) for (let i = 0; i < sorted.length; i++) sorted[i].angle -= (over * (i + 1)) / sorted.length;
  }
  return sorted;
}

export function buildGraph(p: Profile, docs: DocumentRow[], name: string) {
  const nodes: GNode[] = [{ id: "me", kind: "me", label: name, x: 0, y: 0, r: 34, angle: 0 }];
  const edges: Edge[] = [];
  const at = (angle: number, radius: number) => ({ x: Math.cos(angle) * radius, y: Math.sin(angle) * radius, angle });

  const roles = [...(p.experience ?? [])].sort((a, b) => (ym(a.start) ?? 0) - (ym(b.start) ?? 0));
  roles.forEach((role, i) => {
    const angle = -Math.PI / 2 + (TAU * i) / Math.max(roles.length, 1) + (roles.length === 2 ? Math.PI / 4 : 0);
    const s = span(role);
    const years = s ? (s[1] - s[0] + 1) / 12 : 1;
    const id = `role-${i}`;
    nodes.push({ id, kind: "role", label: role.title, sub: role.company, r: Math.min(15, 8 + years * 1.1), role, ...at(angle, R.role) });
    edges.push({ a: "me", b: id, kind: "spine" });
    role.highlights.slice(0, 4).forEach((h, j, all) => {
      const off = (j - (all.length - 1) / 2) * 0.32;
      const lid = `${id}-leaf-${j}`;
      const base = at(angle, R.role);
      nodes.push({ id: lid, kind: "leaf", label: h, text: h, r: 3, angle, x: base.x + Math.cos(angle + off) * 48, y: base.y + Math.sin(angle + off) * 48 });
      edges.push({ a: id, b: lid, kind: "leaf" });
    });
  });
  const roleNodes = nodes.filter((n) => n.kind === "role");

  // Skills sit near the roles that used them.
  const skills = spread(
    p.stacks.map((st, i) => {
      const users = roleNodes.filter((rn) => rn.role!.skills.some((s) => related(s, st.name)));
      return { st, i, users, want: users.length ? circMean(users.map((u) => u.angle)) : (TAU * i) / Math.max(p.stacks.length, 1) };
    }),
    0.2,
  );
  skills.forEach(({ st, i, users, angle }, k) => {
    const id = `skill-${i}`;
    const depthR = { strong: 7.5, working: 6, basic: 4.5, none: 3.5 }[st.depth] ?? 5;
    nodes.push({ id, kind: "skill", label: st.name, stack: st, r: depthR, ...at(angle, R.skill + (k % 2 ? 22 : -10)) });
    if (users.length) users.forEach((u) => edges.push({ a: u.id, b: id, kind: "skill" }));
    else edges.push({ a: "me", b: id, kind: "loose" });
  });

  // Industries, from the profile and from each role.
  const inds = new Map<string, { label: string; users: GNode[] }>();
  for (const label of p.industries) inds.set(keyOf(label), { label, users: [] });
  for (const rn of roleNodes) {
    const ind = rn.role!.industry;
    if (!ind) continue;
    const k = keyOf(ind);
    const hit = [...inds.entries()].find(([key]) => key === k || key.includes(k) || k.includes(key));
    if (hit) hit[1].users.push(rn);
    else inds.set(k, { label: ind, users: [rn] });
  }
  spread(
    [...inds.values()].map((v, i, all) => ({ ...v, want: v.users.length ? circMean(v.users.map((u) => u.angle)) : (TAU * i) / all.length + 0.3 })),
    0.32,
  ).forEach((v, i) => {
    const id = `ind-${i}`;
    nodes.push({ id, kind: "industry", label: v.label, r: 5, ...at(v.angle, R.industry) });
    if (v.users.length) v.users.forEach((u) => edges.push({ a: u.id, b: id, kind: "industry" }));
    else edges.push({ a: "me", b: id, kind: "loose" });
  });

  // Sources close to the centre, with evidence links to the skills they contributed.
  const ready = docs.filter((d) => d.status === "ready");
  ready.forEach((d, i) => {
    const id = `src-${d.id}`;
    nodes.push({ id, kind: "source", label: d.name, doc: d, r: 5, ...at(Math.PI / 2 + (TAU * i) / Math.max(ready.length, 1) + 0.4, R.source) });
    edges.push({ a: "me", b: id, kind: "source" });
    for (const sk of nodes.filter((n) => n.kind === "skill")) {
      if (d.extracted?.stacks.some((s) => related(s.name, sk.label))) edges.push({ a: id, b: sk.id, kind: "evidence" });
    }
  });

  return { nodes, edges };
}

// ── career density, for the timeline ──

export function yearDensity(roles: Role[]) {
  const spans = roles.map((r) => ({ r, s: span(r) })).filter((x) => x.s) as { r: Role; s: [number, number] }[];
  if (!spans.length) return [];
  const first = Math.floor(Math.min(...spans.map((x) => x.s[0])) / 12);
  const last = new Date().getFullYear();
  const out: { year: number; value: number; roles: number }[] = [];
  for (let y = first; y <= last; y++) {
    let v = 0, n = 0;
    for (const { r, s } of spans) {
      const months = Math.max(0, Math.min(s[1], y * 12 + 11) - Math.max(s[0], y * 12) + 1);
      if (months > 0) {
        n++;
        v += (months / 12) * (1 + 0.15 * r.highlights.length + 0.05 * r.skills.length);
      }
    }
    out.push({ year: y, value: v, roles: n });
  }
  return out;
}
