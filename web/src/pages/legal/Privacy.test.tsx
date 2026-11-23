import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { session } from "../../test/fixtures";
import type { Session } from "../../types";
import { Privacy } from "./Privacy";
import { Terms } from "./Terms";

let current: Session = session();
vi.mock("../../session", () => ({ useSession: () => ({ session: current, refresh: async () => {}, setSession: () => {} }) }));

afterEach(cleanup);

const show = (page: React.ReactNode) => render(<MemoryRouter>{page}</MemoryRouter>);

describe("Privacy Policy", () => {
  it("names the operator, the model in use and where data goes", () => {
    current = session();
    const { container } = show(<Privacy />);
    const text = container.textContent ?? "";
    expect(text).toContain("Example Operator (\"we\", \"us\") runs this site");
    expect(text).toContain("Test Model");
    expect(text).toContain("models.example.com");
    expect(text).toContain("Not secure");
    expect(text).toContain("Personal Data Protection Act 2012");
    expect(text).toContain("Personal Data Protection Commission");
  });

  it("shows the efforts made to protect data and the benefits of encryption", () => {
    current = session();
    const text = show(<Privacy />).container.textContent ?? "";
    expect(text).toContain("7. How we protect your data");
    expect(text).toContain("Encrypted at rest, with a key for each person.");
    expect(text).toContain("a key made for you alone");
    expect(text).toContain("A copied database is unreadable.");
    expect(text).toContain("Opening the database shows scrambled values.");
    expect(text).toContain("Reading it takes the master key as well");
    expect(text).toContain("Deleting your account destroys your key.");
    expect(text).toContain("we never sell your data");
  });

  it("is honest about what encryption does not do", () => {
    current = session();
    const text = show(<Privacy />).container.textContent ?? "";
    expect(text).toContain("This is not end-to-end encryption.");
    expect(text).toContain("could technically read it");
    expect(text).toContain("job titles and company names");
    expect(text).toContain("and the keys that go with them");
    // No claim that the operator is locked out, which the design does not deliver.
    expect(text).not.toMatch(/only you can|nobody can read|no one can read|cannot be read by us|not even (we|us)|even we (cannot|can't)|we cannot (see|read)/i);
  });

  it("sends people to the Your data page, not a dashboard", () => {
    current = session();
    const { container } = show(<Privacy />);
    expect(screen.getByRole("link", { name: "Your data" }).getAttribute("href")).toBe("/account");
    expect(container.textContent).not.toMatch(/dashboard/i);
  });

  it("uses generic wording for a jurisdiction it doesn't know", () => {
    current = session({ legal: { ...session().legal, jurisdiction: "Atlantis" } });
    const { container } = show(<Privacy />);
    expect(container.textContent).toContain("your data-protection regulator.");
    expect(container.textContent).not.toContain("Singapore");
  });

  it("warns when the operator hasn't filled in their details", () => {
    current = session({ legal: { ...session().legal, contact: "", operator: "" } });
    const { container } = show(<Privacy />);
    expect(screen.getByText(/hasn.t set its legal details yet/)).toBeTruthy();
    expect(container.textContent).toContain("The operator of this site (\"we\", \"us\") is responsible");
  });

  it("mentions the birthday only when the operator asks Google for it", () => {
    current = session();
    expect(show(<Privacy />).container.textContent).not.toContain("birthday");
    cleanup();
    current = session({ legal: { ...session().legal, birthday: true } });
    expect(show(<Privacy />).container.textContent).toContain("birthday");
  });
});

describe("Terms of Service", () => {
  it("names AceMate, the operator and the governing law", () => {
    current = session();
    const { container } = show(<Terms />);
    expect(container.textContent).toContain("AceMate Terms of Service");
    expect(container.textContent).toContain("governed by the laws of Singapore");
  });
});
