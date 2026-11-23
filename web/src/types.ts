// Mirrors internal/model (Go). Keep in sync.
export type AspectKey = "tech" | "industry" | "people" | "hr";

export const ASPECTS: { key: AspectKey; label: string; weight: number; color: string }[] = [
  { key: "tech", label: "Tech stack", weight: 0.35, color: "var(--c-tech)" },
  { key: "industry", label: "Industry", weight: 0.2, color: "var(--c-ind)" },
  { key: "people", label: "The people", weight: 0.2, color: "var(--c-ppl)" },
  { key: "hr", label: "HR questions", weight: 0.25, color: "var(--c-hr)" },
];

export interface Stack { name: string; depth: "strong" | "working" | "basic" | "none"; years: number; lastUsed: number }
export interface Flag { kind: "plus" | "risk"; text: string }
export interface Role {
  title: string; company: string; start: string; end: string; // "YYYY-MM" or "YYYY"; end "" = current
  industry: string; skills: string[]; highlights: string[];
}
export interface Profile {
  displayName: string; headline: string; location: string; workRights: string[];
  stacks: Stack[]; industries: string[]; flags: Flag[]; experience?: Role[];
}
export interface DocumentRow {
  id: string;
  kind: "pdf" | "docx" | "text" | "link";
  name: string;
  url: string;
  status: "processing" | "ready" | "failed";
  error: string;
  extracted: Profile | null;
  verification: Verification | null;
  createdAt: string;
}
export interface QuizItem { q: string; options: string[]; correct: number; why: string }
export interface TestDef { kind: "none" | "quiz" | "scenario"; quiz: QuizItem[]; prompt: string; rubric: string[] }
export interface JourneyNode {
  id: string; aspect: AspectKey; title: string; baseline: number; links: string[]; milestone: boolean; verify: boolean;
  brief: string; bridge: { from: string; to: string }[]; sections: { title: string; items: string[] }[];
  tasks: string[]; test: TestDef; project: { repo: string; stack: string; milestones: string[] };
  hrQuestions: { q: string; tip: string }[]; peopleCards: boolean; reading: { title: string; url: string }[];
}
export interface Journey {
  meta: { title: string; company: string; band: string; location: string; workType: string; closes: string; summary: string; mustHave: string[]; niceToHave: string[] };
  fit: { match: number; strengths: string[]; gaps: string[]; risks: string[] };
  nodes: JourneyNode[];
  pending?: string[]; // placeholders still being written
  failed?: string[]; // placeholders that couldn't be written (can be retried)
}
export type Progress = Record<string, boolean>;
export type PersonCard = { name?: string; work?: string; fit?: string };
export type People = Record<string, PersonCard[]>;

export interface SessionUser {
  email: string;
  name: string;
  givenName: string;
  image: string;
  termsAccepted: boolean;
  marketingOptIn: boolean;
}
export interface Engine {
  enabled: boolean;
  provider?: string;
  model?: string; // the exact model id, echoed back when the user confirms it
  displayName?: string;
  host?: string; // where resume data is sent
  privacy?: "secure" | "not_secure";
  vision?: boolean; // can read page images (OCR)
  id?: string; // set on entries of `options`: sent back to choose this model
  options?: Engine[]; // present when the operator configured several models the user can pick from
  defaultId?: string; // the pre-selected entry of `options`
}
export interface Legal {
  version: string;
  effective: string; // YYYY-MM-DD
  operator: string;
  contact: string;
  hosting: string;
  jurisdiction: string;
  minAge: number;
  birthday: boolean; // the operator asks Google for the birthday
}
export interface Session {
  user: SessionUser | null;
  providers: { id: string; name: string }[];
  devLogin: boolean;
  models: { documents: Engine; journeys: Engine }; // the model each tool uses (may be the same)
  legal: Legal;
  version?: string; // the server's release version
}
export interface JobCard {
  id: string; title: string; company: string; status: "pending" | "ready" | "failed"; createdAt: string;
  overall: number; aspects: { key: AspectKey; label: string; score: number }[];
}
export interface Dashboard { profile: { data: Profile } | null; jobs: JobCard[]; average: number; journeys: number }
export interface Job {
  id: string; url: string; title: string; status: "pending" | "ready" | "failed"; error: string;
  journey: Journey | null; progress: Progress; people: People;
}

export interface Verification {
  methods: ("text" | "visual" | "pdf")[];
  agreement: number; // 0-100, or -1 when only one reading ran
  confirmed: string[];
  onlyText: string[];
  onlyVisual: string[];
  dropped: string[];
  mismatches: string[];
  notes: string[];
}

/** Whether two tools use the same model at the same host. */
export const sameModel = (a: Engine, b: Engine) => a.enabled && b.enabled && a.model === b.model && a.host === b.host;

/** The model a tool will use: the user's pick from `options`, else the pre-selected one, else the tool's own default. */
export function resolveEngine(e: Engine, choice: string): Engine {
  const list = e.options ?? [];
  return list.find((o) => o.id === choice) ?? list.find((o) => o.id === e.defaultId) ?? e;
}
