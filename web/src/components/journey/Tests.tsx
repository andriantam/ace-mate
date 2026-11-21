import { useEffect, useState } from "react";
import { type JourneyNode } from "../../types";
import { Md } from "./Md";

// The check at the end of a milestone: a quiz, or a scenario to answer in your own words.

export function Quiz({ n, passed, onPass }: { n: JourneyNode; passed: boolean; onPass: () => void }) {
  const [answers, setAnswers] = useState<Record<number, number>>({});
  useEffect(() => {
    if (!passed && n.test.quiz.length && n.test.quiz.every((q, i) => answers[i] === q.correct)) onPass();
  }, [answers, n, passed, onPass]);
  return (
    <>
      <h3>Test {passed ? <span className="pass">· passed ✓</span> : null}</h3>
      <div className="quiz">
        {n.test.quiz.map((q, qi) => (
          <div key={qi}>
            <div className="q"><Md text={q.q} /></div>
            {q.options.map((opt, oi) => {
              const chosen = answers[qi] === oi;
              return (
                <button
                  key={oi}
                  className={`opt ${chosen ? (oi === q.correct ? "right" : "wrong") : ""}`}
                  onClick={() => setAnswers((s) => ({ ...s, [qi]: oi }))}
                >
                  {opt}
                </button>
              );
            })}
            {answers[qi] !== undefined ? (
              <div className="why">{answers[qi] === q.correct ? "Correct. " : "Not quite. "}{q.why}</div>
            ) : null}
          </div>
        ))}
      </div>
    </>
  );
}

export function Scenario({ n, passed, setPassed }: { n: JourneyNode; passed: boolean; setPassed: (v: boolean) => void }) {
  const [ticks, setTicks] = useState<boolean[]>(() => n.test.rubric.map(() => passed));
  function tick(i: number, v: boolean) {
    const next = ticks.map((t, k) => (k === i ? v : t));
    setTicks(next);
    const ok = next.filter(Boolean).length / next.length >= 0.8;
    if (ok !== passed) setPassed(ok);
  }
  return (
    <>
      <h3>Scenario test {passed ? <span className="pass">· passed ✓</span> : null}</h3>
      <div className="card" style={{ fontSize: 14 }}><Md text={n.test.prompt} /></div>
      <textarea placeholder="Type your answer as if you're in the room… (stays on this device)" style={{ marginTop: 8 }} />
      <p className="muted" style={{ fontSize: 13, margin: "8px 0 4px" }}>Self-check against the rubric. Tick 80% or more to pass:</p>
      {n.test.rubric.map((r, i) => (
        <label key={i} className="task">
          <input type="checkbox" checked={ticks[i]} onChange={(e) => tick(i, e.target.checked)} />
          <span>{r}</span>
        </label>
      ))}
    </>
  );
}
