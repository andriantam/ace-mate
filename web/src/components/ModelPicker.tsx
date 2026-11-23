import type { Engine } from "../types";
import { PrivacyPill } from "./AddDocument";

/**
 * Lets the user choose which AI model runs the analysis. Shown only when the operator
 * configured more than one model; otherwise it renders nothing.
 */
export function ModelPicker({ engine, value, onChange }: { engine: Engine; value: string; onChange: (id: string) => void }) {
  const options = engine.options ?? [];
  if (options.length < 2) return null;
  const selected = value || engine.defaultId || "";
  return (
    <fieldset className="modelpick">
      <legend className="f">AI model</legend>
      <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
        {options.map((o) => (
          <label key={o.id} className={`modelopt${selected === o.id ? " on" : ""}`}>
            <input type="radio" name="ai-model" value={o.id} checked={selected === o.id} onChange={() => onChange(o.id ?? "")} />
            <span>
              <b>{o.displayName || o.model}</b> <PrivacyPill privacy={o.privacy} />
              {o.host ? <span className="muted" style={{ display: "block", fontSize: 12 }}>{o.host}</span> : null}
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}
