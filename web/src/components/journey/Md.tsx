/** Renders `code` spans in model text without using innerHTML. */
export function Md({ text }: { text: string }) {
  const parts = text.split(/`([^`]+)`/g);
  return <>{parts.map((p, i) => (i % 2 ? <code key={i}>{p}</code> : p))}</>;
}
