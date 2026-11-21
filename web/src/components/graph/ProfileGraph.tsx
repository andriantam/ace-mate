import { useEffect, useMemo, useRef, useState } from "react";
import type { DocumentRow, Profile } from "../../types";
import { buildGraph, R, roleActiveIn, type GNode } from "./layout";
import { approach, idleTilt, nearest, offsetFor, toBody, type Offset, type Point } from "./motion";

// The profile map. Layout comes from ./layout; motion from ./motion. Pan by dragging, zoom
// by scrolling, click a node for its details. The map floats while idle and reacts to the
// pointer (see motion.ts). Animation writes straight to the SVG elements every frame
// rather than through React state, so it stays smooth on large profiles.

const VIEW = { x: -540, y: -500, w: 1080, h: 1000 };
const REST: Offset = { dx: 0, dy: 0, s: 1 };

function useReducedMotion() {
  const [reduced, setReduced] = useState(() => typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches);
  useEffect(() => {
    if (typeof matchMedia !== "function") return;
    const mq = matchMedia("(prefers-reduced-motion: reduce)");
    const on = () => setReduced(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return reduced;
}

export function ProfileGraph({
  profile,
  docs,
  name,
  image,
  selected,
  onSelect,
  year,
}: {
  profile: Profile;
  docs: DocumentRow[];
  name: string;
  image: string;
  selected: GNode | null;
  onSelect: (n: GNode | null) => void;
  year: number | null;
}) {
  const { nodes, edges } = useMemo(() => buildGraph(profile, docs, name), [profile, docs, name]);
  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const [view, setView] = useState({ k: 1, x: 0, y: 0 });
  const [show, setShow] = useState({ skill: true, industry: true, source: true, leaf: true });
  const [hover, setHover] = useState<string | null>(null);
  const reduced = useReducedMotion();

  const svgRef = useRef<SVGSVGElement>(null);
  const nodeEls = useRef(new Map<string, SVGGElement>());
  const edgeEls = useRef<(SVGLineElement | null)[]>([]);
  const drag = useRef<{ x: number; y: number; vx: number; vy: number; moved: boolean } | null>(null);
  const pointer = useRef<Point | null>(null); // in map units
  const tilt = useRef<Point>({ x: 0, y: 0 }); // pointer direction from the centre, [-1, 1]
  const viewRef = useRef(view);
  const hoverRef = useRef(hover);
  useEffect(() => {
    viewRef.current = view;
    hoverRef.current = hover;
  });

  const visible = (n: GNode) => n.kind === "me" || n.kind === "role" || show[n.kind as keyof typeof show];
  const bodies = useMemo(() => nodes.filter((n) => n.kind === "me" || n.kind === "role" || show[n.kind as keyof typeof show]).map(toBody), [nodes, show]);

  // Scroll (or pinch on a trackpad) zooms toward the pointer. The listener is added by hand
  // because React's wheel handlers are passive and can't stop the page from scrolling.
  useEffect(() => {
    const el = svgRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const ctm = el.getScreenCTM();
      if (!ctm) return;
      const pt = new DOMPoint(e.clientX, e.clientY).matrixTransform(ctm.inverse());
      const factor = Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0015));
      setView((v) => {
        const k = Math.min(3, Math.max(0.5, v.k * factor));
        const f = k / v.k;
        return { k, x: pt.x - (pt.x - v.x) * f, y: pt.y - (pt.y - v.y) * f };
      });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  // The animation loop. Each node's group gets a transform; each edge follows its two ends.
  useEffect(() => {
    if (reduced) {
      // Still: no drift, tilt or push. Hover is shown by the ring and label alone.
      for (const el of nodeEls.current.values()) el.removeAttribute("transform");
      if (svgRef.current) svgRef.current.style.transform = "";
      return;
    }
    const cur = new Map<string, Offset>();
    let raf = 0;
    const start = performance.now();
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame);
      const t = (now - start) / 1000;
      const target = pointer.current ? tilt.current : idleTilt(t);
      const tl = { x: target.x, y: target.y };
      const focus = drag.current?.moved ? null : hoverRef.current;
      for (const b of bodies) {
        const next = approach(cur.get(b.id) ?? REST, offsetFor(b, t, drag.current?.moved ? null : pointer.current, tl, focus), 0.1);
        cur.set(b.id, next);
        const el = nodeEls.current.get(b.id);
        const n = byId.get(b.id);
        if (el && n) el.setAttribute("transform", scaleAt(n, next, next.s));
      }
      edges.forEach((e, i) => {
        const el = edgeEls.current[i];
        if (!el) return;
        const a = byId.get(e.a)!, b = byId.get(e.b)!;
        const oa = cur.get(e.a) ?? REST, ob = cur.get(e.b) ?? REST;
        el.setAttribute("x1", String(a.x + oa.dx));
        el.setAttribute("y1", String(a.y + oa.dy));
        el.setAttribute("x2", String(b.x + ob.dx));
        el.setAttribute("y2", String(b.y + ob.dy));
      });
      // A slight 3D tilt of the whole map toward the pointer (or the idle wander).
      const svg = svgRef.current;
      if (svg) svg.style.transform = `perspective(1400px) rotateX(${(-tl.y * 5).toFixed(2)}deg) rotateY(${(tl.x * 7).toFixed(2)}deg)`;
    };
    raf = requestAnimationFrame(frame);
    return () => cancelAnimationFrame(raf);
  }, [bodies, edges, byId, reduced]);

  // What to emphasise: the selection and its neighbours, the roles active in the chosen
  // year, or (lightly) the neighbours of the node under the pointer.
  const focus = useMemo(() => {
    const around = (id: string) => {
      const set = new Set([id]);
      for (const e of edges) {
        if (e.a === id) set.add(e.b);
        if (e.b === id) set.add(e.a);
      }
      return set;
    };
    if (selected) {
      const set = around(selected.id);
      if (selected.kind === "skill") {
        // Also light the path back to you through the roles that used it.
        for (const e of edges) if (e.b === selected.id && byId.get(e.a)?.kind === "role") set.add("me");
      }
      return { set, strength: 0.16 };
    }
    if (year !== null) {
      const set = new Set(["me"]);
      for (const n of nodes) {
        if (n.kind === "role" && roleActiveIn(n.role!, year)) {
          set.add(n.id);
          for (const e of edges) if (e.a === n.id) set.add(e.b);
        }
      }
      return { set, strength: 0.16 };
    }
    if (hover) return { set: around(hover), strength: 0.45 };
    return null;
  }, [selected, year, hover, edges, nodes, byId]);

  const dim = (id: string) => (focus && !focus.set.has(id) ? focus.strength : 1);

  // Screen position to map units (undoing the viewBox, pan and zoom).
  const toMap = (clientX: number, clientY: number): Point | null => {
    const ctm = svgRef.current?.getScreenCTM();
    if (!ctm) return null;
    const pt = new DOMPoint(clientX, clientY).matrixTransform(ctm.inverse());
    const v = viewRef.current;
    return { x: (pt.x - v.x) / v.k, y: (pt.y - v.y) / v.k };
  };

  const track = (clientX: number, clientY: number) => {
    const svg = svgRef.current;
    if (!svg) return;
    const box = svg.getBoundingClientRect();
    tilt.current = {
      x: Math.max(-1, Math.min(1, ((clientX - box.left) / box.width) * 2 - 1)),
      y: Math.max(-1, Math.min(1, ((clientY - box.top) / box.height) * 2 - 1)),
    };
    const p = toMap(clientX, clientY);
    pointer.current = p;
    const k = viewRef.current.k;
    const id = drag.current?.moved ? null : nearest(bodies, p, 46 / k);
    if (id !== hoverRef.current) setHover(id);
  };

  return (
    <div className="graph-wrap">
      <div className="graph-tools" role="toolbar" aria-label="Graph view">
        <button type="button" onClick={() => setView((v) => ({ ...v, k: Math.max(0.5, v.k / 1.25) }))} aria-label="Zoom out">−</button>
        <button type="button" onClick={() => setView((v) => ({ ...v, k: Math.min(3, v.k * 1.25) }))} aria-label="Zoom in">+</button>
        <button type="button" onClick={() => setView({ k: 1, x: 0, y: 0 })} aria-label="Reset view">⟲</button>
        <span className="sep" />
        {(
          [
            ["skill", "Skills"],
            ["industry", "Industries"],
            ["leaf", "Highlights"],
            ["source", "Sources"],
          ] as const
        ).map(([k, label]) => (
          <button key={k} type="button" className={`chiptoggle ${k} ${show[k] ? "on" : ""}`} aria-pressed={show[k]} onClick={() => setShow((s) => ({ ...s, [k]: !s[k] }))}>
            <i /> {label}
          </button>
        ))}
      </div>

      <svg
        ref={svgRef}
        className="graph"
        viewBox={`${VIEW.x} ${VIEW.y} ${VIEW.w} ${VIEW.h}`}
        role="img"
        aria-label={`Graph of ${name}'s roles, skills, industries and sources`}
        onPointerDown={(e) => {
          drag.current = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y, moved: false };
          (e.target as Element).setPointerCapture?.(e.pointerId);
        }}
        onPointerMove={(e) => {
          const d = drag.current;
          if (d) {
            const ctm = svgRef.current?.getScreenCTM();
            const dx = (e.clientX - d.x) / (ctm?.a || 1), dy = (e.clientY - d.y) / (ctm?.d || 1);
            if (Math.abs(dx) + Math.abs(dy) > 4) d.moved = true;
            if (d.moved) setView((v) => ({ ...v, x: d.vx + dx, y: d.vy + dy }));
          }
          track(e.clientX, e.clientY);
        }}
        onPointerLeave={() => {
          pointer.current = null;
          setHover(null);
        }}
        onPointerUp={(e) => {
          const d = drag.current;
          drag.current = null;
          if (d && !d.moved && (e.target as Element).closest(".gnode") === null) {
            // A click on empty space near a node picks that node; far from any, it clears.
            const n = hoverRef.current ? byId.get(hoverRef.current) : undefined;
            onSelect(n && n.id !== selected?.id ? n : null);
          }
        }}
      >
        <defs>
          <radialGradient id="core" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#fff6dc" stopOpacity="0.95" />
            <stop offset="35%" stopColor="#f3c979" stopOpacity="0.55" />
            <stop offset="100%" stopColor="#f3c979" stopOpacity="0" />
          </radialGradient>
          <filter id="glow" x="-200%" y="-200%" width="500%" height="500%">
            <feGaussianBlur stdDeviation="3.2" result="b" />
            <feMerge><feMergeNode in="b" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <clipPath id="me-clip"><circle r="32" /></clipPath>
        </defs>

        <g transform={`translate(${view.x} ${view.y}) scale(${view.k})`}>
          {[R.role, R.skill, R.industry].map((r) => (
            <circle key={r} r={r} className="ring" />
          ))}
          <circle r="140" fill="url(#core)" opacity="0.5" pointerEvents="none" className="core" />

          <g className="edges">
            {edges.map((e, i) => {
              const a = byId.get(e.a)!, b = byId.get(e.b)!;
              if (!visible(a) || !visible(b)) return null;
              if (e.kind === "evidence" && !(selected && (selected.id === e.a || selected.id === e.b))) return null;
              return (
                <line
                  key={i}
                  ref={(el) => {
                    edgeEls.current[i] = el;
                  }}
                  x1={a.x}
                  y1={a.y}
                  x2={b.x}
                  y2={b.y}
                  className={`edge ${e.kind}`}
                  opacity={Math.min(dim(e.a), dim(e.b))}
                />
              );
            })}
          </g>

          {nodes.filter(visible).map((n) => {
            const sel = selected?.id === n.id;
            const hot = hover === n.id;
            const c = Math.cos(n.angle);
            const anchor = n.kind === "me" ? "middle" : c > 0.25 ? "start" : c < -0.25 ? "end" : "middle";
            const off = n.r + 6;
            const lx = n.kind === "me" ? 0 : n.x + Math.cos(n.angle) * off;
            const ly = n.kind === "me" ? n.r + 20 : n.y + Math.sin(n.angle) * off + (Math.abs(c) <= 0.25 ? (Math.sin(n.angle) > 0 ? 10 : -4) : 4);
            const showLabel = n.kind !== "leaf" || sel || hot;
            return (
              <g
                key={n.id}
                ref={(el) => {
                  if (el) nodeEls.current.set(n.id, el);
                  else nodeEls.current.delete(n.id);
                }}
                className={`gnode ${n.kind}${sel ? " sel" : ""}${hot ? " hot" : ""}`}
                opacity={dim(n.id)}
                tabIndex={n.kind === "leaf" ? -1 : 0}
                role="button"
                aria-label={`${n.kind === "me" ? "You" : n.kind}: ${n.label}`}
                onClick={(e) => {
                  e.stopPropagation();
                  onSelect(sel ? null : n);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onSelect(sel ? null : n);
                  }
                }}
              >
                {n.kind === "me" ? (
                  <>
                    <circle r={n.r + 12} className="halo" />
                    <circle r={n.r + 2} className="me-ring" filter="url(#glow)" />
                    {image ? (
                      <image href={image} x={-32} y={-32} width={64} height={64} clipPath="url(#me-clip)" preserveAspectRatio="xMidYMid slice" />
                    ) : (
                      <text className="initials" dy="0.35em">{name.split(/\s+/).map((w) => w[0]).slice(0, 2).join("").toUpperCase() || "?"}</text>
                    )}
                  </>
                ) : (
                  <circle cx={n.x} cy={n.y} r={n.r} className="dot" filter={n.kind === "leaf" ? undefined : "url(#glow)"} />
                )}
                {hot && !sel && n.kind !== "me" ? <circle cx={n.x} cy={n.y} r={n.r + 7} className="hover-ring" /> : null}
                {sel && n.kind !== "me" ? (
                  <g className="lens" transform={`translate(${n.x} ${n.y})`}>
                    <circle r={n.r + 16} />
                    <circle r={n.r + 24} className="ticks" />
                  </g>
                ) : null}
                {showLabel ? (
                  <text x={lx} y={ly} textAnchor={anchor} className="glabel">
                    {n.label.length > 34 ? n.label.slice(0, 33) + "…" : n.label}
                    {n.sub ? <tspan x={lx} dy="1.2em" className="gsub">{n.sub.length > 32 ? n.sub.slice(0, 31) + "…" : n.sub}</tspan> : null}
                  </text>
                ) : null}
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}

/** A transform that moves a node by o and scales it by s around its own centre. */
function scaleAt(n: GNode, o: Offset, s: number) {
  const x = n.x + o.dx, y = n.y + o.dy;
  return `translate(${x.toFixed(2)} ${y.toFixed(2)}) scale(${s.toFixed(3)}) translate(${-n.x} ${-n.y})`;
}
