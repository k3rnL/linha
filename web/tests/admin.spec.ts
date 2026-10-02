import { test, expect, type Page } from "@playwright/test";
const now = "2026-10-01T12:00:00Z";
const context = {
  id: "context",
  owner: "owner",
  name: "metoc-extract",
  version: "config-version",
  state: "READY",
  createdAt: now,
  observedAt: now,
  metricContext: "metoc",
  liveClients: 1,
  queuedJobs: 0,
  runningJobs: 2,
  readyWorkers: 1,
  desiredInstances: 1,
  spec: {
    image: "worker@sha256:example",
    engine: {
      type: "spark",
      version: "3.5.6",
      settings: { drivers: { minDrivers: 1, maxDrivers: 2 } },
    },
    results: { type: "s3", destination: "results" },
  },
};
const job = {
  id: "original",
  owner: "owner",
  contextId: "context",
  contextName: "metoc-extract",
  contextVersion: "config-version",
  metricContext: "metoc",
  state: "RUNNING",
  submittedAt: now,
  updatedAt: now,
  attemptCount: 1,
  request: {
    handler: "extract",
    version: 1,
    payload: { count: 10 },
    idempotencyKey: "original-key",
    retry: { maxAttempts: 1, backoffSeconds: 1 },
  },
};
async function fixtures(page: Page, role = "operator") {
  await page.route("**/ui/config", (r) =>
    r.fulfill({ json: { securityEnabled: true, loginURL: "/ui/oidc/login" } }),
  );
  await page.route("**/v1/admin/**", async (r) => {
    const url = new URL(r.request().url());
    let value: unknown = { items: [] };
    if (url.pathname === "/v1/admin/session")
      value = { actor: "admin", role, csrf: "csrf" };
    else if (url.pathname === "/v1/admin/contexts")
      value = { items: [context] };
    else if (url.pathname === "/v1/admin/contexts/context") value = context;
    else if (url.pathname === "/v1/admin/jobs/original") value = job;
    else if (url.pathname === "/v1/admin/jobs") value = { items: [job] };
    else if (url.pathname === "/v1/admin/jobs/created")
      value = {
        ...job,
        id: "created",
        state: "QUEUED",
        replayedFrom: "original",
      };
    await r.fulfill({ json: value });
  });
}
test("context observations do not renew client leases", async ({ page }) => {
  await fixtures(page);
  const mutations: string[] = [];
  page.on("request", (r) => {
    if (r.method() !== "GET") mutations.push(r.url());
  });
  await page.goto("/ui/contexts");
  await expect(page.getByRole("link", { name: "config-versi" })).toBeVisible();
  await page.getByRole("link", { name: "config-versi" }).click();
  await expect(page.getByText("No current engine instances.")).toBeVisible();
  await page.getByRole("tab", { name: "Clients", exact: true }).click();
  await expect(page.getByText("No stored client leases.")).toBeVisible();
  expect(mutations).toEqual([]);
});
test("edited replay uses a fresh key and keeps it across a lost response", async ({
  page,
}) => {
  await fixtures(page);
  await page.addInitScript(() =>
    Object.defineProperty(crypto, "randomUUID", { value: undefined }),
  );
  const bodies: Record<string, unknown>[] = [];
  await page.route("**/v1/admin/contexts/context/jobs", async (r) => {
    const body = r.request().postDataJSON();
    bodies.push(body);
    if (bodies.length === 1) {
      await r.abort("failed");
      return;
    }
    await r.fulfill({
      status: 202,
      json: {
        ...job,
        id: "created",
        state: "QUEUED",
        request: body,
        replayedFrom: "original",
      },
    });
  });
  await page.goto("/ui/requests/new?sourceJobId=original");
  await expect(page.getByLabel("JSON payload")).toHaveValue(
    '{\n  "count": 10\n}',
  );
  await page.getByLabel("JSON payload").fill('{"count":20}');
  await page.getByRole("button", { name: "Submit replay" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await page.getByRole("button", { name: "Submit replay" }).click();
  await expect(page).toHaveURL(/requests\/created$/);
  expect(bodies).toHaveLength(2);
  expect(bodies[0]).toEqual(bodies[1]);
  expect(bodies[0].idempotencyKey).not.toBe("original-key");
  expect(bodies[0].payload).toEqual({ count: 20 });
  expect(bodies[0].replayedFrom).toBe("original");
  expect(bodies[0]).not.toHaveProperty("deadline");
});
test("viewer navigation hides mutations", async ({ page }) => {
  await fixtures(page, "viewer");
  await page.goto("/ui/requests/original");
  await expect(
    page.getByRole("heading", { name: "Execution", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Edit and replay" })).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("button", { name: "Cancel request" }),
  ).toHaveCount(0);
  await page.goto("/ui/requests/new");
  await expect(page.getByText("Operator access is required")).toBeVisible();
});
test("cancellation stays pending and does not stop a driver", async ({
  page,
}) => {
  await fixtures(page);
  let cancelling = false;
  const mutations: string[] = [];
  page.on("request", (r) => {
    if (r.method() !== "GET") mutations.push(new URL(r.url()).pathname);
  });
  await page.route("**/v1/admin/jobs/original", (r) =>
    r.fulfill({
      json: { ...job, state: cancelling ? "CANCELLING" : "RUNNING" },
    }),
  );
  await page.route("**/v1/admin/jobs/original/cancel", (r) => {
    cancelling = true;
    return r.fulfill({ json: { ...job, state: "CANCELLING" } });
  });
  page.on("dialog", (d) => d.accept());
  await page.goto("/ui/requests/original");
  await page.getByRole("button", { name: "Cancel request" }).click();
  await expect(
    page.getByRole("button", { name: "Cancelling…" }),
  ).toBeDisabled();
  expect(mutations).toEqual(["/v1/admin/jobs/original/cancel"]);
});
test("missing and stale Spark UI links remain unavailable", async ({
  page,
}) => {
  await fixtures(page);
  await page.route("**/v1/admin/contexts/context/instances?*", (r) =>
    r.fulfill({
      json: {
        items: [
          {
            id: "driver",
            resourceKind: "sparkapplication",
            resourceUid: "uid",
            state: "READY",
            incarnation: "pod-uid",
            available: true,
            stale: true,
            observedAt: now,
            detail: {
              driverPod: "spark-driver",
              driverPhase: "Running",
              links: [{ name: "spark-ui", url: "https://spark.example.org" }],
            },
          },
        ],
      },
    }),
  );
  await page.goto("/ui/contexts/context");
  await expect(page.getByText("Stale observation")).toBeVisible();
  await expect(
    page.getByText("No current published Spark UI URL"),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Open spark-ui" })).toHaveCount(
    0,
  );
});
test("JSON previews render active content as text", async ({ page }) => {
  await fixtures(page);
  await page.route("**/v1/admin/jobs/original", (r) =>
    r.fulfill({
      json: {
        ...job,
        state: "SUCCEEDED",
        result: { descriptor: { kind: "json" }, expired: false },
      },
    }),
  );
  await page.route("**/v1/admin/jobs/original/result", (r) =>
    r.fulfill({
      json: {
        descriptor: { kind: "json", schema: "result", version: 1 },
        expiresAt: "2027-01-01T00:00:00Z",
        expired: false,
        files: [],
      },
    }),
  );
  await page.route("**/v1/admin/jobs/original/preview", (r) =>
    r.fulfill({
      json: {
        text: '{"html":"<img src=x onerror=window.compromised=true>"}',
        truncated: false,
        validJSON: true,
        maxBytes: 65536,
      },
    }),
  );
  await page.goto("/ui/requests/original");
  await page.getByRole("button", { name: "Preview JSON (64 KiB)" }).click();
  await expect(
    page.locator("pre").filter({ hasText: "onerror" }),
  ).toBeVisible();
  expect(await page.evaluate(() => "compromised" in window)).toBe(false);
});

test("replay preserves JVM-sized integer payloads", async ({ page }) => {
  await fixtures(page);
  await page.route("**/v1/admin/jobs/original", (r) =>
    r.fulfill({
      contentType: "application/json",
      body: JSON.stringify(job).replace(
        '"count":10',
        '"count":9223372036854775807',
      ),
    }),
  );
  let body = "";
  await page.route("**/v1/admin/contexts/context/jobs", (r) => {
    body = r.request().postData() ?? "";
    return r.fulfill({ status: 202, json: { ...job, id: "created" } });
  });
  await page.goto("/ui/requests/new?sourceJobId=original");
  await expect(page.getByLabel("JSON payload")).toHaveValue(
    '{\n  "count": 9223372036854775807\n}',
  );
  await page.getByRole("button", { name: "Submit replay" }).click();
  await expect(page).toHaveURL(/requests\/created$/);
  expect(body).toContain('"count":9223372036854775807');
});

test("valid Spark UI stays usable when allocation data is incomplete", async ({
  page,
}) => {
  await fixtures(page);
  await page.route("**/v1/admin/contexts/context/instances?*", (r) =>
    r.fulfill({
      json: {
        items: [
          {
            id: "instance",
            contextId: "context",
            state: "READY",
            resourceKind: "sparkapplication",
            resourceUid: "current",
            incarnation: "driver",
            available: false,
            stale: false,
            observedAt: new Date().toISOString(),
            detail: {
              condition: "missing_resources",
              links: [
                { name: "spark-ui", url: "https://spark.example/current" },
              ],
            },
          },
        ],
      },
    }),
  );
  await page.goto("/ui/contexts/context");
  await expect(page.getByText("Allocation data incomplete")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Open spark-ui" }),
  ).toHaveAttribute("href", "https://spark.example/current");
});

const overview = {
  observedAt: now,
  since: "2026-09-30T12:00:00Z",
  runningJobs: 12,
  queuedJobs: 128,
  retryingJobs: 3,
  cancellingJobs: 1,
  failedJobs24h: 4,
  succeededJobs24h: 219,
  cancelledJobs24h: 2,
  activeContexts: 6,
  liveClients: 9,
  readyWorkers: 4,
  workerSlots: 16,
  occupiedSlots: 12,
  serversReady: 2,
  serversUnready: 0,
  serversStale: 0,
  meanProcessingSeconds24h: 82,
  oldestQueuedSeconds: 142,
  activeContextDetails: [
    { ...context, condition: "", queuedJobs: 128, runningJobs: 12 },
  ],
  recentFailures: [
    {
      id: "original",
      contextId: "context",
      contextName: "metoc-extract",
      owner: "owner",
      handler: "extract",
      message: "Input dataset is unavailable",
      updatedAt: now,
    },
  ],
};
test("overview is the default page and uses complete server aggregates", async ({
  page,
}) => {
  await fixtures(page);
  await page.route("**/v1/admin/overview", (r) =>
    r.fulfill({ json: overview }),
  );
  const mutations: string[] = [];
  page.on("request", (r) => {
    if (r.method() !== "GET") mutations.push(r.url());
  });
  await page.goto("/ui/");
  await expect(
    page.getByRole("heading", { name: "Overview", exact: true }),
  ).toBeVisible();
  const queued = page
    .locator(".overview-stat")
    .filter({ hasText: "Queued now" });
  await expect(queued.locator("strong")).toHaveText("128");
  await expect(page.getByText("Input dataset is unavailable")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Browse all failed requests" }),
  ).toHaveAttribute("href", "/ui/requests?state=FAILED");
  await page.screenshot({
    path: "/tmp/linha-refine-overview.png",
    fullPage: true,
  });
  await page.getByRole("link", { name: "Browse all failed requests" }).click();
  await expect(page).toHaveURL(/requests\?state=FAILED$/);
  expect(mutations).toEqual([]);
});

test("overview distinguishes loading, errors and an empty installation", async ({
  page,
}) => {
  await fixtures(page, "viewer");
  let release: () => void = () => {};
  const waiting = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/v1/admin/overview", async (r) => {
    await waiting;
    return r.fulfill({
      status: 403,
      json: { error: { code: "FORBIDDEN", message: "Overview unavailable" } },
    });
  });
  await page.goto("/ui/");
  await expect(page.getByText("Loading overview…")).toBeVisible();
  await expect(page.locator(".overview-stat")).toHaveCount(0);
  release();
  await expect(page.getByRole("alert")).toContainText("Overview unavailable");
  await expect(page.locator(".overview-stat")).toHaveCount(0);
  await page.route("**/v1/admin/overview", (r) =>
    r.fulfill({
      json: {
        ...overview,
        runningJobs: 0,
        queuedJobs: 0,
        retryingJobs: 0,
        cancellingJobs: 0,
        failedJobs24h: 0,
        succeededJobs24h: 0,
        cancelledJobs24h: 0,
        activeContexts: 0,
        liveClients: 0,
        readyWorkers: 0,
        workerSlots: 0,
        occupiedSlots: 0,
        serversReady: 1,
        meanProcessingSeconds24h: null,
        oldestQueuedSeconds: 0,
        activeContextDetails: [],
        recentFailures: [],
      },
    }),
  );
  await page.reload();
  await expect(page.getByText("No completed executions")).toBeVisible();
  await expect(page.getByText("Queue empty", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("link", { name: "New request", exact: true }),
  ).toHaveCount(0);
});

test("current allocations stay separate from lazily loaded history and client hostnames", async ({
  page,
}) => {
  await fixtures(page);
  const instance = {
    id: "current-instance",
    contextId: "context",
    resourceKind: "sparkapplication",
    resourceUid: "app-current",
    incarnation: "driver-current",
    state: "READY",
    available: true,
    stale: false,
    observedAt: now,
    detail: {
      application: "linha-current",
      applicationState: "RUNNING",
      driverPod: "linha-current-driver",
      driverPhase: "Running",
      executorStates: { Running: 2 },
      links: [{ name: "spark-ui", url: "https://spark.example.org/current" }],
      resources: {
        driver_cpu_request: 2,
        driver_cpu_limit: 2,
        driver_memory_request: 6442450944,
        driver_memory_limit: 6442450944,
        executor_cpu_request: 4,
        executor_cpu_limit: 4,
        executor_memory_request: 11811160064,
        executor_memory_limit: 11811160064,
      },
    },
  };
  let historyReads = 0;
  await page.route("**/v1/admin/contexts/context/instances?*", (r) => {
    const historical =
      new URL(r.request().url()).searchParams.get("state") === "DEAD";
    if (historical) historyReads++;
    return r.fulfill({
      json: {
        items: [
          historical
            ? {
                ...instance,
                id: "retired",
                state: "DEAD",
                detail: {
                  ...instance.detail,
                  driverPod: "linha-retired-driver",
                },
              }
            : instance,
        ],
      },
    });
  });
  await page.route("**/v1/admin/contexts/context/clients?*", (r) =>
    r.fulfill({
      json: {
        items: [
          {
            id: "lease-1",
            clientId: "client-1",
            hostname: "api-pod-a",
            state: "active",
            expiresAt: now,
          },
          {
            id: "lease-2",
            clientId: "client-2",
            state: "expired",
            expiresAt: now,
          },
        ],
      },
    }),
  );
  await page.goto("/ui/contexts/context");
  await expect(
    page.getByRole("heading", { name: "Current resource allocation" }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Memory limit" }),
  ).toBeVisible();
  await expect(
    page.getByRole("row", { name: /Driver 2 cores 2 cores 6.0 GiB 6.0 GiB/ }),
  ).toBeVisible();
  expect(historyReads).toBe(0);
  await page.screenshot({
    path: "/tmp/linha-refine-instances.png",
    fullPage: true,
  });
  await page.locator(".instance-history > summary").click();
  await expect(
    page.getByText("linha-retired-driver", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Historical resource allocation" }),
  ).not.toBeVisible();
  await page.locator(".retired-instance > summary").click();
  await expect(
    page.getByRole("heading", { name: "Historical resource allocation" }),
  ).toBeVisible();
  await expect(
    page
      .locator(".retired-instance")
      .getByRole("link", { name: "Open spark-ui" }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "Clients", exact: true }).click();
  const client = page.getByRole("row").filter({ hasText: "client-1" });
  await expect(client.locator("strong")).toHaveText("api-pod-a");
  await expect(client).toContainText("client-1");
  await expect(page.getByText("Hostname not reported")).toBeVisible();
});
