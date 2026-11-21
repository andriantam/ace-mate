import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { docs, profile } from "../test/fixtures";
import { ProfileTree } from "./ProfileTree";

afterEach(cleanup);

describe("ProfileTree (phones)", () => {
  it("lists roles newest first, skills by depth, industries and sources", () => {
    const { container } = render(<ProfileTree profile={profile} docs={docs} />);
    const roles = [...container.querySelectorAll(".t-main b")].map((b) => b.textContent);
    expect(roles.slice(0, 2)).toEqual(["Senior Engineer", "Engineer"]);
    expect(screen.getByText("Strong")).toBeTruthy();
    expect(screen.getAllByText("Payments").length).toBeGreaterThan(0);
    expect(screen.getByText("resume.pdf")).toBeTruthy();
  });

  it("opens the career and strong skills, and keeps the rest collapsed", () => {
    const { container } = render(<ProfileTree profile={profile} docs={docs} />);
    const top = [...container.querySelectorAll(".ptree > details")] as HTMLDetailsElement[];
    expect(top[0].open).toBe(true); // career
    expect(top.slice(2).every((d) => !d.open)).toBe(true);
  });
});
