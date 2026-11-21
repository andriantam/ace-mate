import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { docs, profile } from "../../test/fixtures";
import { ProfileGraph } from "./ProfileGraph";

afterEach(cleanup);

const renderGraph = (onSelect = vi.fn()) =>
  render(<ProfileGraph profile={profile} docs={docs} name="Test Person" image="" selected={null} onSelect={onSelect} year={null} />);

describe("ProfileGraph", () => {
  it("draws you, your roles and your skills as focusable nodes", () => {
    renderGraph();
    expect(screen.getByRole("button", { name: "You: Test Person" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "role: Senior Engineer" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "skill: Go" })).toBeTruthy();
  });

  it("selects a node by click or keyboard", () => {
    const onSelect = vi.fn();
    renderGraph(onSelect);
    const go = screen.getByRole("button", { name: "skill: Go" });
    fireEvent.click(go);
    expect(onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ label: "Go", kind: "skill" }));
    fireEvent.keyDown(screen.getByRole("button", { name: "role: Engineer" }), { key: "Enter" });
    expect(onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ label: "Engineer", kind: "role" }));
  });

  it("hides a layer from the toolbar", () => {
    renderGraph();
    const toggle = screen.getByRole("button", { name: /Skills/ });
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(toggle);
    expect(screen.queryByRole("button", { name: "skill: Go" })).toBeNull();
    expect(screen.getByRole("button", { name: "role: Engineer" })).toBeTruthy();
  });

  it("stays still for people who prefer reduced motion", () => {
    const mm = vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() });
    vi.stubGlobal("matchMedia", mm);
    const raf = vi.spyOn(window, "requestAnimationFrame");
    renderGraph();
    expect(raf).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});
