import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  use: { baseURL: "http://127.0.0.1:5182", headless: true },
  webServer: {
    command: "npm run dev -- --port 5182",
    url: "http://127.0.0.1:5182/ui/",
    reuseExistingServer: !process.env.CI,
  },
});
