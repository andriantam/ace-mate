export function Ring({ value, size = 58, color = "var(--accent)", stroke = 4 }: { value: number; size?: number; color?: string; stroke?: number }) {
  const r = size / 2 - stroke;
  const c = 2 * Math.PI * r;
  const v = Math.min(1, Math.max(0, value));
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden="true">
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--line)" strokeWidth={stroke} />
      <circle
        cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round"
        strokeDasharray={c} strokeDashoffset={c * (1 - v)} transform={`rotate(-90 ${size / 2} ${size / 2})`}
        style={{ transition: "stroke-dashoffset .6s" }}
      />
    </svg>
  );
}
