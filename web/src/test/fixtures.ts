import type { DocumentRow, Engine, Legal, Profile, Session } from "../types";

// Neutral, made-up data for tests only. Never shipped in the app.

export const profile: Profile = {
  displayName: "Test Person",
  headline: "Backend engineer",
  location: "Remote",
  workRights: [],
  industries: ["Payments"],
  flags: [{ kind: "plus", text: "Led a migration" }],
  stacks: [
    { name: "Go", depth: "strong", years: 5, lastUsed: 2026 },
    { name: "PostgreSQL", depth: "working", years: 3, lastUsed: 2025 },
    { name: "Rust", depth: "basic", years: 1, lastUsed: 2024 },
  ],
  experience: [
    { title: "Engineer", company: "Example Co", start: "2019-01", end: "2021-12", industry: "Payments", skills: ["Go"], highlights: ["Built an API"] },
    { title: "Senior Engineer", company: "Sample Ltd", start: "2022-01", end: "", industry: "Payments", skills: ["Go", "PostgreSQL"], highlights: [] },
  ],
};

export const docs: DocumentRow[] = [
  {
    id: "d1", kind: "pdf", name: "resume.pdf", url: "", status: "ready", error: "", createdAt: "2026-01-01T00:00:00Z",
    extracted: { ...profile, stacks: profile.stacks.slice(0, 2) }, verification: null,
  },
];

export const engine: Engine = { enabled: true, provider: "openai", model: "test-model", displayName: "Test Model", host: "models.example.com", privacy: "not_secure" };

export const legal: Legal = {
  version: "2", effective: "2026-11-21", operator: "Example Operator", contact: "privacy@example.com", hosting: "Example Cloud",
  jurisdiction: "Singapore", minAge: 18, birthday: false,
};

export const session = (over: Partial<Session> = {}): Session => ({
  user: { email: "person@example.com", name: "Test Person", givenName: "Test", image: "", termsAccepted: true, marketingOptIn: false },
  providers: [{ id: "google", name: "Google" }],
  devLogin: false,
  models: { documents: engine, journeys: engine },
  legal,
  ...over,
});
