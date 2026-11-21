import type { UploadEvent } from "./api";
import type { Verification } from "./types";

// Turns real upload and processing events into progress for the loader. The bar only moves
// to a milestone when the event that proves it arrives; between events it may creep toward
// the next milestone (see useCreep) so it never looks stuck.

export const STAGES = ["Uploading file", "Scanning", "Analysing", "Synthesising", "Done"] as const;

export type Run = {
  stage: number; // index into STAGES
  target: number; // progress (0-100) proven by events
  ceiling: number; // the bar may creep up to here while waiting
  message: string;
  log: string[];
  expected: number; // readings running
  finished: number; // readings finished
  next: number; // server events consumed (for polling)
  error?: string;
  result?: Verification | null;
  done: boolean;
};

export const freshRun = (link = false): Run => ({
  stage: 0,
  target: 0,
  ceiling: link ? 20 : 18,
  message: link ? "Sending the link…" : "Uploading file…",
  log: [],
  expected: 1,
  finished: 0,
  next: 0,
  done: false,
});

/** The log line for a "retrying" event. */
export function retryLine(ev: { detail?: string; attempt?: number; of?: number; seconds?: number }): string {
  const wait = ev.seconds ? ` Trying again in ${ev.seconds < 60 ? `${ev.seconds} seconds` : `${Math.round(ev.seconds / 60)} min`}` : " Trying again";
  return `${ev.detail ?? "That didn't work."}${wait} (attempt ${ev.attempt ?? 2} of ${ev.of ?? 3}).`;
}

const READING = { text: "Text reading", visual: "Visual (OCR) reading", pdf: "PDF reading" } as const;

export function advance(r: Run, ev: UploadEvent, model: string): Run {
  const next: Run = { ...r, log: [...r.log] };
  const say = (m: string) => {
    next.message = m;
    next.log.push(m);
  };
  switch (ev.type) {
    case "upload": {
      const f = ev.loaded / Math.max(ev.total, 1);
      next.target = Math.round(2 + 18 * f);
      next.ceiling = 20;
      next.message = f >= 1 ? "Upload complete. Waiting for the server…" : `Uploading file… ${Math.round(100 * f)}%`;
      return next;
    }
    case "done":
      next.stage = 4;
      next.target = next.ceiling = 100;
      next.done = true;
      next.result = ev.verification;
      say("Done. Added to your profile.");
      return next;
    case "error":
      next.done = true;
      next.error = ev.error;
      say(ev.error);
      return next;
  }
  switch (ev.step) {
    case "received":
      next.stage = 1;
      next.target = 22;
      next.ceiling = 30;
      say(
        ev.kind === "link" ? "Link received." : ev.kind === "docx" ? "File received. Reading the Word document…" : ev.kind === "text" ? "Text received." : "File received. Scanning the PDF…",
      );
      break;
    case "fetch":
      next.stage = 1;
      next.target = 24;
      next.ceiling = 34;
      say(`Scanning ${ev.detail ?? "the page"}…`);
      break;
    case "fetched":
      next.target = 34;
      next.ceiling = 38;
      say(`Read ${ev.detail ?? "the page"} (${Math.round((ev.chars ?? 0) / 1000)}k characters).`);
      break;
    case "extract":
      next.stage = 1;
      next.target = Math.max(next.target, 28);
      next.ceiling = 38;
      if (ev.detail !== "text received") {
        say(ev.ok ? "Found the text layer." : "No text layer found. This looks like a scan, so it will be read from the page images (OCR).");
      }
      break;
    case "render":
      next.target = 36;
      next.ceiling = 40;
      say(ev.ok ? `Rendered ${ev.pages} page${ev.pages === 1 ? "" : "s"} as images for OCR.` : "Couldn't render the pages for OCR. Continuing with the text only.");
      break;
    case "read": {
      const m = ev.methods ?? ["text"];
      next.stage = 2;
      next.expected = m.length;
      next.finished = 0;
      next.target = 40;
      next.ceiling = 40 + 45 / m.length - 3;
      say(
        m.includes("text") && m.includes("visual")
          ? `${model} is reading it two ways at once: the text, and the page images (OCR).`
          : m.includes("visual")
            ? `${model} is reading the page images (OCR).`
            : `${model} is analysing it.`,
      );
      break;
    }
    case "read_done":
      next.finished = r.finished + 1;
      next.target = Math.min(85, 40 + (45 * next.finished) / r.expected);
      next.ceiling = next.finished >= r.expected ? 87 : next.target + 45 / r.expected - 3;
      say(`${READING[ev.method ?? "text"]} ${ev.ok ? "finished" : "failed"}.`);
      break;
    case "reconcile":
      next.stage = 3;
      next.target = 88;
      next.ceiling = 93;
      say("Synthesising: cross-checking the two readings against each other and the text.");
      break;
    case "retrying":
      say(retryLine(ev));
      break;
    case "saving":
      next.stage = 3;
      next.target = 95;
      next.ceiling = 98;
      say("Synthesising: merging it into your profile.");
      break;
  }
  return next;
}
