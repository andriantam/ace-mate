import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useDocs, type Pending } from "../docs";
import { STAGES, type Run } from "../progress";
import { useSession } from "../session";
import { resolveEngine, type DocumentRow, type Engine, type Verification } from "../types";
import { ModelPicker } from "./ModelPicker";

export function PrivacyPill({ privacy }: { privacy?: Engine["privacy"] }) {
  const secure = privacy === "secure";
  return (
    <span
      className={`pill ${secure ? "secure" : "insecure"}`}
      title={secure ? "Data stays on our own server, or the provider's terms say inputs aren't used to train models." : "A free or unverified service: it may keep, log or train on what you send."}
    >
      {secure ? "Secure" : "Not secure"}
    </span>
  );
}

const KIND_LABEL: Record<DocumentRow["kind"], string> = { pdf: "PDF", docx: "Word", text: "Text", link: "Link" };

/** A progress bar that shows proven progress and creeps toward the next milestone while waiting. */
export function RunBar({ run, started, link, stages = STAGES }: { run: Run; started: number; link?: boolean; stages?: readonly string[] }) {
  const [shown, setShown] = useState(run.target);
  const [now, setNow] = useState(() => Date.now());
  const ref = useRef(run);
  useEffect(() => {
    ref.current = run;
  });
  useEffect(() => {
    if (run.done) {
      setShown(run.error ? Math.max(ref.current.target, 4) : 100);
      return;
    }
    const id = window.setInterval(() => {
      const r = ref.current;
      setShown((s) => Math.max(r.target, s + Math.max(0, r.ceiling - Math.max(s, r.target)) * 0.04));
      setNow(Date.now());
    }, 300);
    return () => window.clearInterval(id);
  }, [run.done, run.error]);
  const pct = Math.round(run.done && !run.error ? 100 : shown);
  return (
    <>
      <ol className="stages">
        {stages.map((s, i) => (
          <li key={s} data-stage={i === 0 && link ? "Sending link" : s} className={run.error && i === run.stage ? "fail" : i < run.stage || (run.done && !run.error) ? "done" : i === run.stage ? "now" : ""}>
            {i === 0 && link ? "Sending link" : s}
          </li>
        ))}
      </ol>
      <div className="pbar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
        <i style={{ width: `${pct}%` }} className={run.error ? "fail" : run.done ? "ok" : ""} />
      </div>
      <div className="row" style={{ justifyContent: "space-between", fontSize: 12.5, marginTop: 6, gap: 10 }}>
        <span className={run.error ? "errtext" : ""}>{run.message}</span>
        <span className="muted mono" style={{ whiteSpace: "nowrap" }}>
          {pct}%{run.done ? "" : ` · ${Math.max(0, Math.round((now - started) / 1000))}s`}
        </span>
      </div>
    </>
  );
}

function Outcome({ v }: { v: Verification | null | undefined }) {
  if (!v) return null;
  const both = v.methods.length > 1;
  return (
    <div className="outcome">
      {both ? <b>Read two ways, {v.agreement}% agreement.</b> : <b>Read once (no cross-check).</b>}
      {v.confirmed.length ? <span> Confirmed: {v.confirmed.slice(0, 8).join(", ")}{v.confirmed.length > 8 ? "…" : ""}.</span> : null}
      {v.dropped.length ? <span className="errtext"> Removed as unverified: {v.dropped.map((d) => d.split(" (")[0]).join(", ")}.</span> : null}
      {v.mismatches.length ? <span> {v.mismatches.length} disagreement{v.mismatches.length === 1 ? "" : "s"} (text value kept).</span> : null}
    </div>
  );
}

function PendingItem({ p, onDismiss }: { p: Pending; onDismiss: () => void }) {
  const [started] = useState(() => Date.now());
  return (
    <li className="docrun">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <span className="docname"><span className="kind">{KIND_LABEL[p.kind]}</span> {p.name}</span>
        {p.run.done ? <button type="button" className="linkbtn" onClick={onDismiss}>Clear</button> : null}
      </div>
      <RunBar run={p.run} started={started} link={p.kind === "link"} />
      {p.run.done && !p.run.error ? <Outcome v={p.run.result} /> : null}
    </li>
  );
}

export function AddDocumentModal() {
  const { session } = useSession();
  const { open, setOpen, add, pending, dismiss, docs, remove } = useDocs();
  const [mode, setMode] = useState<"file" | "link" | "paste">("file");
  const [file, setFile] = useState<File | null>(null);
  const [url, setUrl] = useState("");
  const [text, setText] = useState("");
  const [ack, setAck] = useState(false);
  const [choice, setChoice] = useState(""); // the model the user picked; "" is the pre-selected one
  const [confirmDel, setConfirmDel] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, setOpen]);

  if (!open || !session) return null;
  const engine = resolveEngine(session.models.documents, choice);
  const model = engine.displayName || engine.model || "The AI model";
  const validUrl = /^https:\/\/[^\s/]+\.[^\s]+/.test(url.trim());
  const ready = ack && (mode === "file" ? Boolean(file) : mode === "link" ? validUrl : text.trim().length >= 100);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!ready || !engine.model) return;
    if (mode === "file" && file) {
      const lower = file.name.toLowerCase();
      add({ file, engine: engine.id, ackModel: engine.model, name: file.name, kind: lower.endsWith(".pdf") ? "pdf" : lower.endsWith(".docx") ? "docx" : "text" });
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
    } else if (mode === "link") {
      add({ url: url.trim(), engine: engine.id, ackModel: engine.model, name: url.trim().replace(/^https:\/\/(www\.)?/, ""), kind: "link" });
      setUrl("");
    } else {
      add({ text, engine: engine.id, ackModel: engine.model, name: "Pasted notes", kind: "text" });
      setText("");
    }
  };

  const busyKeys = new Set(pending.map((p) => p.id));
  const saved = docs.filter((d) => !busyKeys.has(d.id) || d.status !== "processing");

  return (
    <div className="modal-scrim" onMouseDown={(e) => e.target === e.currentTarget && setOpen(false)}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="adddoc-title">
        <button type="button" className="modal-close" onClick={() => setOpen(false)} aria-label="Close">×</button>
        <h2 id="adddoc-title">Add to your profile</h2>
        <p className="muted" style={{ marginTop: 0, fontSize: 13 }}>
          Your resume, other documents, or links such as your GitHub profile or personal site. Each one adds to your profile.
        </p>

        {!engine.enabled ? (
          <p className="warnline">This site has no AI model configured, so documents can&apos;t be read. See the README to set one up.</p>
        ) : (
          <form onSubmit={submit}>
            <div className="seg" role="tablist">
              {(["file", "link", "paste"] as const).map((m) => (
                <button key={m} type="button" role="tab" aria-selected={mode === m} className={mode === m ? "on" : ""} onClick={() => setMode(m)}>
                  {m === "file" ? "Upload a file" : m === "link" ? "Add a link" : "Paste text"}
                </button>
              ))}
            </div>

            {mode === "file" ? (
              <div className="dropzone">
                <input
                  ref={fileRef}
                  type="file"
                  accept=".pdf,.docx,.txt,.md,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,text/plain,text/markdown"
                  onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                />
                <p className="muted">
                  PDF, Word (.docx) or text, up to 5 MB.{" "}
                  {engine.vision ? "PDFs are read twice, from the text and from the page images (OCR), and cross-checked." : "This model can't read images, so scanned PDFs can't be read."}
                </p>
              </div>
            ) : mode === "link" ? (
              <div className="dropzone">
                <input type="url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://github.com/your-name or https://your-site.com" />
                <p className="muted">Public https pages only. GitHub profiles and repositories are read through GitHub&apos;s API; other pages as text.</p>
              </div>
            ) : (
              <div className="dropzone">
                <textarea value={text} onChange={(e) => setText(e.target.value)} placeholder="Paste a resume, a project write-up or notes about your experience." maxLength={60000} />
                <p className="muted">At least 100 characters.</p>
              </div>
            )}

            <ModelPicker engine={session.models.documents} value={choice} onChange={(id) => { setChoice(id); setAck(false); }} />

            <div className="consent-box">
              <label className="tick">
                <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} />
                <span>
                  <b>{model}</b> <PrivacyPill privacy={engine.privacy} /> will be scanning your {mode === "link" ? "page" : "document"}.
                  {engine.host ? <span className="muted"> It is sent to <code>{engine.host}</code>.</span> : null}
                </span>
              </label>
              {engine.privacy !== "secure" ? (
                <p className="warnline">This is not a secure model: the service may keep, log or train on what you send. Remove anything you wouldn&apos;t want shared.</p>
              ) : null}
              <p className="muted" style={{ fontSize: 12, margin: "8px 0 0" }}>
                ⓘ How your data is processed fully depends on the AI model. How we manage and store your data securely (only the AI summary and whatever you provide) is described in our{" "}
                <Link to="/privacy" target="_blank" rel="noopener">Privacy Policy</Link>.
              </p>
            </div>

            <div className="row" style={{ justifyContent: "flex-end", marginTop: 12 }}>
              <button className="btn" disabled={!ready}>Add</button>
            </div>
          </form>
        )}

        {pending.length ? (
          <>
            <h3 className="modal-sub">In progress</h3>
            <p className="muted" style={{ fontSize: 12, margin: "-4px 0 8px" }}>You can close this window. Processing carries on in the background.</p>
            <ul className="docruns">
              {[...pending].reverse().map((p) => (
                <PendingItem key={p.key} p={p} onDismiss={() => dismiss(p.key)} />
              ))}
            </ul>
          </>
        ) : null}

        {saved.length ? (
          <>
            <h3 className="modal-sub">Your documents ({saved.length})</h3>
            <ul className="doclist">
              {saved.map((d) => (
                <li key={d.id}>
                  <span className="docname">
                    <span className="kind">{KIND_LABEL[d.kind]}</span> {d.url ? <a href={d.url} target="_blank" rel="noopener noreferrer">{d.name}</a> : d.name}
                  </span>
                  <span className={`status ${d.status}`} title={d.error || undefined}>{d.status === "ready" ? "added" : d.status}</span>
                  {confirmDel === d.id ? (
                    <span className="row" style={{ gap: 6 }}>
                      <button type="button" className="linkbtn danger" onClick={() => void remove(d.id).finally(() => setConfirmDel(""))}>Remove</button>
                      <button type="button" className="linkbtn" onClick={() => setConfirmDel("")}>Keep</button>
                    </span>
                  ) : (
                    <button type="button" className="linkbtn" onClick={() => setConfirmDel(d.id)} aria-label={`Remove ${d.name}`}>Remove</button>
                  )}
                </li>
              ))}
            </ul>
            <p className="muted" style={{ fontSize: 12 }}>Removing a document takes what it added out of your profile.</p>
          </>
        ) : null}
      </div>
    </div>
  );
}
