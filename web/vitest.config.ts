import { defineConfig } from "vitest/config";

// Unit and component tests: `npm test`. Kept apart from vite.config.ts because Vitest
// bundles its own Vite.
export default defineConfig({
  test: { environment: "jsdom", include: ["src/**/*.test.{ts,tsx}"], restoreMocks: true },
});
