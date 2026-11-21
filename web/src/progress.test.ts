import { describe, expect, it } from "vitest";
import type { UploadEvent } from "./api";
import { advance, freshRun, retryLine } from "./progress";

describe("document progress", () => {
  it("moves only forward through the stages to done", () => {
    let r = freshRun();
    const seen: number[] = [];
    const events: UploadEvent[] = [
      { type: "upload", loaded: 1, total: 1 },
      { type: "progress", step: "received", kind: "pdf" },
      { type: "progress", step: "read", methods: ["text", "visual"] },
      { type: "progress", step: "read_done", method: "text", ok: true },
      { type: "progress", step: "read_done", method: "visual", ok: true },
      { type: "progress", step: "reconcile" },
      { type: "progress", step: "saving" },
      { type: "done", verification: null },
    ];
    for (const ev of events) {
      r = advance(r, ev, "Test Model");
      seen.push(r.target);
    }
    expect(seen).toEqual([...seen].sort((a, b) => a - b));
    expect(r).toMatchObject({ done: true, target: 100, stage: 4 });
    expect(r.log.some((l) => l.includes("Test Model is reading it two ways"))).toBe(true);
  });

  it("explains retries and errors", () => {
    let r = advance(freshRun(), { type: "progress", step: "retrying", detail: "The AI service is busy.", attempt: 2, of: 3, seconds: 20 }, "M");
    expect(r.message).toBe("The AI service is busy. Trying again in 20 seconds (attempt 2 of 3).");
    expect(r.done).toBe(false);
    r = advance(r, { type: "error", error: "Couldn't read that document." }, "M");
    expect(r).toMatchObject({ done: true, error: "Couldn't read that document." });
  });

  it("words long waits in minutes", () => {
    expect(retryLine({ detail: "Busy.", seconds: 120, attempt: 3, of: 3 })).toBe("Busy. Trying again in 2 min (attempt 3 of 3).");
  });
});
