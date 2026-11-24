import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resolveEngine, type Engine } from "../types";
import { ModelPicker } from "./ModelPicker";

const claude: Engine = { enabled: true, id: "claude", model: "claude-haiku-5-5", displayName: "Claude Haiku 5.5", host: "api.anthropic.com", privacy: "secure" };
const or: Engine = { enabled: true, id: "openrouter", model: "x/y:free", displayName: "Y", host: "openrouter.ai", privacy: "not_secure" };
const both: Engine = { ...claude, options: [claude, or], defaultId: "claude" };

afterEach(cleanup);

describe("ModelPicker", () => {
  it("renders nothing with a single model", () => {
    const { container } = render(<ModelPicker engine={claude} value="" onChange={() => {}} />);
    expect(container.innerHTML).toBe("");
  });

  it("lists both models with their privacy label and reports the pick", () => {
    const onChange = vi.fn();
    render(<ModelPicker engine={both} value="" onChange={onChange} />);
    expect(screen.getByText("Secure")).toBeTruthy();
    expect(screen.getByText("Not secure")).toBeTruthy();
    fireEvent.click(screen.getByLabelText(/^Y/));
    expect(onChange).toHaveBeenCalledWith("openrouter");
  });

  it("resolves the pick, falling back to the pre-selected model", () => {
    expect(resolveEngine(both, "openrouter").id).toBe("openrouter");
    expect(resolveEngine(both, "").id).toBe("claude");
    expect(resolveEngine(claude, "").id).toBe("claude");
  });
});
