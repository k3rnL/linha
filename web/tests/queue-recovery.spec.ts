import { test, expect, type Page } from "@playwright/test";

test.use({ timezoneId: "Europe/Paris" });
const now = "2026-10-15T12:00:00Z";
const source = {
  id: "old-context",
  owner: "application",
  name: "sample-ingest",
  version: "old-version",
  state: "DRAINING",
  createdAt: now,
  observedAt: now,
  metricContext: "sample-ingest",
  liveClients: 0,
  queuedJobs: 2,
  runningJobs: 1,
  readyWorkers: 0,
  desiredInstances: 1,
  spec: {
    image: "worker:old",
    engine: { type: "spark", version: "3.5.6", settings: {} },
    results: { type: "s3", destination: "results" },
  },
};
const target = {
  ...source,
  id: "new-context",
  version: "new-version",
  state: "READY",
  liveClients: 1,
  readyWorkers: 1,
};
const makeJob = (id: string, state = "QUEUED") => ({
  id,
  owner: source.owner,
  contextId: source.id,
  contextName: source.name,
  contextVersion: source.version,
  state,
  submittedAt: now,
  updatedAt: now,
  attemptCount: state === "RUNNING" ? 1 : 0,
  request: {
    handler: "ingest",
    version: 1,
    payload: { file: id },
    idempotencyKey: id,
    retry: { maxAttempts: 1, backoffSeconds: 1 },
  },
});
async function fixture(page: Page, role = "operator", partial = false) {
  let jobs = [
    makeJob("queued-one"),
    makeJob("queued-two"),
    makeJob("running", "RUNNING"),
  ];
  const batches: {
    action: string;
    targetContextId?: string;
    jobs: { id: string; contextId: string }[];
  }[] = [];
  const queries: URL[] = [];
  await page.route("**/ui/config", (route) =>
    route.fulfill({
      json: { securityEnabled: true, loginURL: "/ui/oidc/login" },
    }),
  );
  await page.route("**/v1/admin/**", async (route) => {
    const url = new URL(route.request().url());
    let value: unknown = { items: [] };
    if (url.pathname === "/v1/admin/session")
      value = { actor: "operator", role, csrf: "csrf" };
    else if (url.pathname === "/v1/admin/contexts")
      value = {
        items: [
          source,
          target,
          {
            ...target,
            id: "foreign-context",
            owner: "different",
            version: "foreign",
          },
          {
            ...target,
            id: "stopped-context",
            version: "stopped",
            state: "STOPPED",
          },
          {
            ...target,
            id: "archived-context",
            name: "archived-ingest",
            state: "STOPPED",
          },
        ],
      };
    else if (url.pathname === `/v1/admin/contexts/${source.id}`) value = source;
    else if (url.pathname === "/v1/admin/jobs/batch") {
      expect(route.request().headers()["x-linha-csrf"]).toBe("csrf");
      const body = route.request().postDataJSON();
      batches.push(body);
      const items = body.jobs.map(
        (selected: { id: string; contextId: string }) => {
          const job = jobs.find((job) => job.id === selected.id)!;
          if (partial && selected.id === "queued-two") {
            jobs = jobs.map((job) =>
              job.id === selected.id ? { ...job, state: "RUNNING" } : job,
            );
            return {
              id: selected.id,
              outcome: "failed",
              error: {
                code: "CONFLICT",
                message: "Request was claimed; refresh and try again",
              },
            };
          }
          const next = {
            ...job,
            ...(body.action === "move"
              ? { contextId: target.id, contextVersion: target.version }
              : {
                  state: job.state === "RUNNING" ? "CANCELLING" : "CANCELLED",
                }),
          };
          jobs = jobs.map((job) => (job.id === next.id ? next : job));
          return {
            id: selected.id,
            outcome:
              body.action === "move" ? "moved" : next.state.toLowerCase(),
            job: next,
          };
        },
      );
      value = { items };
    } else if (url.pathname === "/v1/admin/jobs") {
      queries.push(url);
      value = {
        items: jobs.filter(
          (job) =>
            !url.searchParams.get("contextId") ||
            job.contextId === url.searchParams.get("contextId"),
        ),
      };
    }
    await route.fulfill({ json: value });
  });
  return { batches, queries };
}

test("global requests move pending selection and retain partial failures", async ({
  page,
}) => {
  const { batches } = await fixture(page, "operator", true);
  await page.goto("/ui/requests");
  await page
    .getByRole("checkbox", { name: "Select request queued-one", exact: true })
    .check();
  await page
    .getByRole("checkbox", { name: "Select request queued-two", exact: true })
    .check();
  await page
    .getByRole("button", { name: "Move selected", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("Search destination").fill("new-version");
  await expect(
    dialog.getByLabel("Destination version").locator("option"),
  ).toHaveCount(2);
  await dialog.getByLabel("Destination version").selectOption(target.id);
  await dialog.getByRole("button", { name: "Confirm move" }).click();
  await expect(page.getByRole("status")).toContainText(
    "1 completed · 1 need attention",
  );
  await expect(page.getByRole("status")).toContainText("Request was claimed");
  await expect(
    page.getByRole("checkbox", {
      name: "Select request queued-two",
      exact: true,
    }),
  ).toBeChecked();
  await expect(
    page.getByRole("checkbox", {
      name: "Select request queued-one",
      exact: true,
    }),
  ).not.toBeChecked();
  await expect(
    page.getByRole("button", { name: "Move selected", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Cancel selected", exact: true }),
  ).toBeEnabled();
  expect(batches).toEqual([
    {
      action: "move",
      targetContextId: target.id,
      jobs: [
        { id: "queued-one", contextId: source.id },
        { id: "queued-two", contextId: source.id },
      ],
    },
  ]);
});

test("context request tab supports the same batch cancellation", async ({
  page,
}) => {
  const { batches } = await fixture(page);
  await page.goto(`/ui/contexts/${source.id}`);
  await page.getByRole("tab", { name: "Requests", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Select all requests on this page" })
    .check();
  await expect(page.getByText("3 selected", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Move selected", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Cancel selected", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm cancellation" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    "3 completed · 0 need attention",
  );
  expect(batches[0].action).toBe("cancel");
  expect(batches[0].jobs).toHaveLength(3);
});

test("context request tab moves a queued request to a corrected version", async ({
  page,
}) => {
  const { batches } = await fixture(page);
  await page.goto(`/ui/contexts/${source.id}`);
  await page.getByRole("tab", { name: "Requests", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Select request queued-one", exact: true })
    .check();
  await page
    .getByRole("button", { name: "Move selected", exact: true })
    .click();
  await page.getByLabel("Destination version").selectOption(target.id);
  await page.getByRole("button", { name: "Confirm move" }).click();
  await expect(page.getByRole("status")).toContainText("1 completed");
  expect(batches[0].targetContextId).toBe(target.id);
  await expect(
    page.getByRole("checkbox", {
      name: "Select request queued-one",
      exact: true,
    }),
  ).toHaveCount(0);
});

test("context suggestions and local dates replace RFC3339 text fields", async ({
  page,
}) => {
  const { queries } = await fixture(page);
  await page.goto("/ui/requests");
  const contextFilter = page.getByRole("combobox", {
    name: "Context name",
    exact: true,
  });
  await contextFilter.click();
  await expect(
    page.getByRole("option", { name: source.name, exact: true }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("option", { name: "archived-ingest", exact: true }),
  ).toBeVisible();
  await page.getByRole("option", { name: source.name, exact: true }).click();
  await expect(contextFilter).toHaveValue(source.name);
  await expect(contextFilter).toHaveAttribute("aria-expanded", "false");
  await page.getByLabel("From", { exact: true }).fill("2026-10-15T14:00");
  await page.getByLabel("Until", { exact: true }).fill("2026-10-15T15:00");
  await expect
    .poll(() => queries.at(-1)?.searchParams.get("until"))
    .toBe("2026-10-15T13:00:00.000Z");
  expect(queries.at(-1)?.searchParams.get("from")).toBe(
    "2026-10-15T12:00:00.000Z",
  );
  expect(queries.at(-1)?.searchParams.get("name")).toBe(source.name);
  await page
    .getByRole("checkbox", { name: "Select request queued-one", exact: true })
    .check();
  await page
    .getByRole("combobox", { name: "State", exact: true })
    .selectOption("QUEUED");
  await expect(page.getByText("0 selected", { exact: true })).toBeVisible();
  await page.getByLabel("Until", { exact: true }).fill("2026-10-15T13:00");
  await expect(page.getByRole("alert")).toContainText(
    "From must be before Until",
  );
  expect(
    queries.every(
      (url) =>
        !url.searchParams.get("from") ||
        !url.searchParams.get("until") ||
        new Date(url.searchParams.get("from")!) <=
          new Date(url.searchParams.get("until")!),
    ),
  ).toBeTruthy();
});

test("viewer cannot select or mutate requests in either view", async ({
  page,
}) => {
  await fixture(page, "viewer");
  await page.goto("/ui/requests");
  await expect(
    page.getByRole("heading", { name: "Requests", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Move selected" })).toHaveCount(
    0,
  );
  await page.goto(`/ui/contexts/${source.id}`);
  await page.getByRole("tab", { name: "Requests", exact: true }).click();
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Cancel selected" }),
  ).toHaveCount(0);
});

test("context picker supports keyboard, free text, dismissal and themed options", async ({
  page,
}) => {
  const { queries } = await fixture(page);
  await page.goto("/ui/requests");
  const filter = page.getByRole("combobox", {
    name: "Context name",
    exact: true,
  });
  await filter.focus();
  const list = page.getByRole("listbox", { name: "Context suggestions" });
  await expect(list).toBeVisible();
  await expect(list).toHaveCSS("background-color", "rgb(255, 255, 255)");
  await page.screenshot({ path: test.info().outputPath("context-picker.png") });
  await filter.press("ArrowDown");
  await filter.press("Enter");
  await expect(filter).toHaveValue("archived-ingest");
  await expect(list).toHaveCount(0);
  await filter.press("ArrowUp");
  await expect(filter).toHaveAttribute("aria-activedescendant", /.+-0$/);
  await filter.press("Escape");
  await expect(filter).toHaveValue("archived-ingest");
  await expect(list).toHaveCount(0);
  await filter.fill("SaMpLe");
  await expect(
    page.getByRole("option", { name: source.name, exact: true }),
  ).toBeVisible();
  await filter.press("ArrowDown");
  await filter.press("Enter");
  await expect(filter).toHaveValue(source.name);
  await filter.fill("custom-name");
  await expect(
    page.getByText("No matching contexts. You can keep this text as a filter."),
  ).toBeVisible();
  await expect
    .poll(() => queries.at(-1)?.searchParams.get("name"))
    .toBe("custom-name");
  await filter.press("Tab");
  await expect(list).toHaveCount(0);
  await page.getByRole("button", { name: "Toggle suggestions" }).click();
  await expect(list).toBeVisible();
  await page.getByRole("heading", { name: "Requests", exact: true }).click();
  await expect(list).toHaveCount(0);
  await filter.fill("");
  await expect(
    page.getByRole("option", { name: "archived-ingest", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("option", { name: source.name, exact: true }),
  ).toBeVisible();
});

for (const viewport of [
  { width: 1600, height: 1000 },
  { width: 390, height: 844 },
  { width: 844, height: 390 },
]) {
  test(`batch dialogs stay centered and usable at ${viewport.width}x${viewport.height}`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await fixture(page);
    await page.goto(`/ui/contexts/${source.id}`);
    await page.getByRole("tab", { name: "Requests", exact: true }).click();
    await page
      .getByRole("checkbox", { name: "Select request queued-one", exact: true })
      .check();
    for (const action of ["Move selected", "Cancel selected"]) {
      await page.getByRole("button", { name: action, exact: true }).click();
      const dialog = page.getByRole("dialog");
      await expect(dialog).toBeVisible();
      const box = await dialog.boundingBox();
      expect(box).not.toBeNull();
      expect(
        Math.abs(box!.x + box!.width / 2 - viewport.width / 2),
      ).toBeLessThan(2);
      expect(
        Math.abs(box!.y + box!.height / 2 - viewport.height / 2),
      ).toBeLessThan(2);
      expect(box!.x).toBeGreaterThanOrEqual(15);
      expect(box!.y).toBeGreaterThanOrEqual(15);
      expect(box!.width).toBeLessThanOrEqual(viewport.width - 30);
      expect(box!.height).toBeLessThanOrEqual(viewport.height - 30);
      await dialog
        .getByRole("button", { name: "Back", exact: true })
        .scrollIntoViewIfNeeded();
      await expect(
        dialog.getByRole("button", { name: "Back", exact: true }),
      ).toBeInViewport();
      await page.screenshot({
        path: test
          .info()
          .outputPath(`${action.split(" ")[0].toLowerCase()}-modal.png`),
      });
      await dialog.getByRole("button", { name: "Back", exact: true }).click();
      await expect(dialog).toHaveCount(0);
    }
  });
}
