import { useRef, useState } from "react";
import { api } from "../../api";
import { type People } from "../../types";

export function PeopleEditor({ jobId, nodeId, initial }: { jobId: string; nodeId: string; initial: People }) {
  const [cards, setCards] = useState(initial[nodeId]?.length ? initial[nodeId] : [{}]);
  const [status, setStatus] = useState("");
  const all = useRef(initial);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  function update(next: typeof cards) {
    setCards(next);
    all.current = { ...all.current, [nodeId]: next };
    setStatus("Saving…");
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(async () => {
      try {
        await api.savePeople(jobId, all.current);
        setStatus("Saved");
      } catch {
        setStatus("Couldn't save");
      }
    }, 600);
  }

  return (
    <>
      <h3>Person cards <span className="muted" style={{ fontSize: 12, fontWeight: 400 }}>{status}</span></h3>
      <p className="muted" style={{ fontSize: 13, marginTop: 0 }}>Fill these in from public, professional sources only. Only you can see them.</p>
      {cards.map((c, i) => (
        <div key={i} className="pcard">
          {(["name", "work", "fit"] as const).map((f) => (
            <input
              key={f}
              type="text"
              maxLength={f === "name" ? 200 : 1000}
              placeholder={{ name: "Name & role", work: "Public work: talks, repos, posts", fit: "Their likely gap → how you fill it" }[f]}
              value={c[f] ?? ""}
              onChange={(e) => update(cards.map((x, k) => (k === i ? { ...x, [f]: e.target.value } : x)))}
              style={{ marginBottom: 6 }}
            />
          ))}
        </div>
      ))}
      {cards.length < 20 ? <button className="btn ghost" onClick={() => update([...cards, {}])}>+ Person</button> : null}
    </>
  );
}
