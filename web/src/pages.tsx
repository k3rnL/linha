import { useEffect, useRef, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  api,
  APIError,
  decodeJSON,
  newSubmissionKey,
  useData,
  usePage,
} from "./api";
import { Button } from "./components/ui/button";
import {
  bytes,
  Card,
  date,
  Detail,
  Empty,
  ErrorMessage,
  Filters,
  Heading,
  JobLink,
  Json,
  Pagination,
  short,
  State,
} from "./components/common";
import type {
  Attempt,
  Audit,
  Client,
  Context,
  Job,
  Page,
  Part,
  Preview,
  Result,
  Server,
  Session,
  Worker,
} from "./types";
import { Instances } from "./instances";
const jobStates = [
  "QUEUED",
  "RETRYING",
  "RUNNING",
  "CANCELLING",
  "SUCCEEDED",
  "FAILED",
  "CANCELLED",
];
const terminal = (state: string) =>
  ["SUCCEEDED", "FAILED", "CANCELLED"].includes(state);
export function Contexts({ operator }: { operator: boolean }) {
  const [search] = useSearchParams();
  const [filters, setFilters] = useState<Record<string, string>>({
    metricContext: search.get("metricContext") ?? "",
    state: search.get("state") ?? "",
  });
  const query = usePage<Context>("/v1/admin/contexts", filters);
  const groups = Map.groupBy(
    query.data?.items ?? [],
    (c) => `${c.owner}/${c.name}`,
  );
  return (
    <>
      <Heading
        eyebrow="Administration"
        title="Contexts"
        description="Live clients and accepted work keep each version running."
        action={
          operator && (
            <Button asChild>
              <Link to="/requests/new">New request</Link>
            </Button>
          )
        }
      />
      <Filters
        values={filters}
        onChange={setFilters}
        fields={[
          { key: "name", label: "Name" },
          { key: "owner", label: "Owner" },
          { key: "engine", label: "Engine" },
          {
            key: "state",
            label: "State",
            options: ["STARTING", "READY", "DRAINING", "STOPPED"],
          },
        ]}
      >
        <label className="checkbox">
          <input
            type="checkbox"
            checked={filters.includeStopped === "true"}
            onChange={(e) =>
              setFilters({
                ...filters,
                includeStopped: String(e.target.checked),
              })
            }
          />
          Include stopped
        </label>
      </Filters>
      <ErrorMessage error={query.error} />
      <div className="card table-card">
        <table>
          <thead>
            <tr>
              <th>Context / version</th>
              <th>State</th>
              <th>Clients</th>
              <th>Requests</th>
              <th>Workers / desired</th>
              <th>Observed</th>
            </tr>
          </thead>
          <tbody>
            {[...groups].flatMap(([key, contexts]) => [
              <tr className="group" key={key}>
                <td colSpan={6}>
                  <strong>{contexts[0].name}</strong>
                  <span>Owner {contexts[0].owner}</span>
                </td>
              </tr>,
              ...contexts.map((c) => (
                <tr key={c.id}>
                  <td>
                    <Link to={`/contexts/${c.id}`}>
                      <span className="mono">{short(c.version)}</span>
                    </Link>
                    <small>
                      {c.spec.engine.type} {c.spec.engine.version}
                    </small>
                  </td>
                  <td>
                    <State value={c.state} />
                    {c.condition && (
                      <small className="warning">{c.condition}</small>
                    )}
                  </td>
                  <td>{c.liveClients}</td>
                  <td>
                    {c.queuedJobs} queued · {c.runningJobs} running
                  </td>
                  <td>
                    {c.readyWorkers} / {c.desiredInstances}
                  </td>
                  <td>{date(c.observedAt)}</td>
                </tr>
              )),
            ])}
          </tbody>
        </table>
        {!query.isPending && !query.data?.items.length && (
          <Empty>No contexts match these filters.</Empty>
        )}
        {query.isPending && <Empty>Loading contexts…</Empty>}
        <Pagination {...query} />
      </div>
    </>
  );
}
function ContextClients({ id }: { id: string }) {
  const q = usePage<Client>(`/v1/admin/contexts/${id}/clients`);
  return (
    <>
      <ErrorMessage error={q.error} />
      <table>
        <thead>
          <tr>
            <th>Client</th>
            <th>State</th>
            <th>Last renewal</th>
            <th>Expires</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.items.map((c) => (
            <tr key={c.id}>
              <td>
                <strong className="client-hostname">
                  {c.hostname || "Hostname not reported"}
                </strong>
                <small className="mono">{c.clientId}</small>
                <small>Lease {short(c.id)}</small>
              </td>
              <td>
                <State value={c.state} />
              </td>
              <td>{date(c.lastRenewedAt)}</td>
              <td>{c.state === "released" ? "Released" : date(c.expiresAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {!q.data?.items.length && <Empty>No stored client leases.</Empty>}
      <Pagination {...q} />
    </>
  );
}
function ContextWorkers({ id }: { id: string }) {
  const q = usePage<Worker>(`/v1/admin/contexts/${id}/workers`);
  return (
    <>
      <ErrorMessage error={q.error} />
      {q.data?.items.map((w) => (
        <details key={w.id} className="instance">
          <summary>
            <span className="mono">{short(w.id)}</span>
            <State value={w.state} />
            <span>
              {w.occupiedSlots} / {w.capacity} slots
            </span>
            <span>Heartbeat {date(w.heartbeatAt)}</span>
          </summary>
          <Json value={w.capabilities} />
        </details>
      ))}
      {!q.data?.items.length && <Empty>No registered workers.</Empty>}
      <Pagination {...q} />
    </>
  );
}
export function ContextDetail({ operator }: { operator: boolean }) {
  const { id } = useParams();
  const q = useData<Context>(`/v1/admin/contexts/${id}`);
  const [tab, setTab] = useState("Instances");
  const c = q.data;
  return (
    <>
      <Heading
        eyebrow="Context version"
        title={c?.name ?? "Context"}
        description={
          c ? `${c.owner} · ${c.spec.engine.type} · ${c.version}` : undefined
        }
        action={
          operator && (
            <Button asChild>
              <Link to={`/requests/new?contextId=${id}`}>New request</Link>
            </Button>
          )
        }
      />
      <ErrorMessage error={q.error} />
      {c && (
        <>
          <div className="stats">
            {[
              ["Lifecycle", <State value={c.state} />],
              ["Live clients", c.liveClients],
              ["Accepted work", c.queuedJobs + c.runningJobs],
              [
                "Ready / desired workers",
                `${c.readyWorkers} / ${c.desiredInstances}`,
              ],
            ].map(([label, value]) => (
              <div className="stat" key={String(label)}>
                <span>{label}</span>
                <strong>{value}</strong>
              </div>
            ))}
          </div>
          <p className="muted">
            Observed {date(c.observedAt)}.{" "}
            {c.liveClients === 0
              ? c.queuedJobs + c.runningJobs > 0
                ? "No live clients. Accepted work finishes before this version stops."
                : "No live clients or accepted work. This version drains regardless of its minimum driver count."
              : "Active client leases keep this version available."}
          </p>
          {c.condition && <div className="warning">{c.condition}</div>}
          <div className="tabs" role="tablist">
            {[
              "Instances",
              "Workers",
              "Clients",
              "Requests",
              "Configuration",
            ].map((t) => (
              <button
                role="tab"
                aria-selected={tab === t}
                onClick={() => setTab(t)}
                key={t}
              >
                {t}
              </button>
            ))}
          </div>
          <div className="card">
            {tab === "Instances" && (
              <Instances id={c.id} engine={c.spec.engine.type} />
            )}{" "}
            {tab === "Workers" && <ContextWorkers id={c.id} />}{" "}
            {tab === "Clients" && <ContextClients id={c.id} />}{" "}
            {tab === "Requests" && <JobsTable contextId={c.id} />}{" "}
            {tab === "Configuration" && <Json value={c.spec} />}
          </div>
        </>
      )}
    </>
  );
}
function JobsTable({
  contextId,
  filters = {},
}: {
  contextId?: string;
  filters?: Record<string, string>;
}) {
  const q = usePage<Job>("/v1/admin/jobs", {
    ...filters,
    ...(contextId ? { contextId } : {}),
  });
  return (
    <>
      <ErrorMessage error={q.error} />
      <table>
        <thead>
          <tr>
            <th>Request</th>
            <th>Context</th>
            <th>Handler</th>
            <th>State / progress</th>
            <th>Attempts</th>
            <th>Submitted</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.items.map((j) => (
            <tr key={j.id}>
              <td>
                <JobLink id={j.id} />
                {j.replayedFrom && <small>Replay</small>}
              </td>
              <td>
                <Link to={`/contexts/${j.contextId}`}>{j.contextName}</Link>
                <small>
                  {short(j.contextVersion)} · {j.owner}
                </small>
              </td>
              <td>
                {j.request.handler}
                <small>Contract v{j.request.version}</small>
              </td>
              <td>
                <State value={j.state} />
                {j.progress && (
                  <small>
                    {Math.round(j.progress.fraction * 100)}% ·{" "}
                    {j.progress.message}
                  </small>
                )}
              </td>
              <td>{j.attemptCount}</td>
              <td>{date(j.submittedAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {!q.isPending && !q.data?.items.length && (
        <Empty>No requests match these filters.</Empty>
      )}
      {q.isPending && <Empty>Loading requests…</Empty>}
      <Pagination {...q} />
    </>
  );
}
export function Requests({ operator }: { operator: boolean }) {
  const [search] = useSearchParams();
  const [filters, setFilters] = useState<Record<string, string>>({
    contextId: search.get("contextId") ?? "",
    metricContext: search.get("metricContext") ?? "",
    state: search.get("state") ?? "",
  });
  return (
    <>
      <Heading
        eyebrow="Execution"
        title="Requests"
        description="Durable status, attempts and published results."
        action={
          operator && (
            <Button asChild>
              <Link to="/requests/new">New request</Link>
            </Button>
          )
        }
      />
      <Filters
        values={filters}
        onChange={setFilters}
        fields={[
          { key: "owner", label: "Owner" },
          { key: "name", label: "Context name" },
          { key: "state", label: "State", options: jobStates },
          { key: "from", label: "From (RFC3339)" },
          { key: "until", label: "Until (RFC3339)" },
        ]}
      />
      <div className="card table-card">
        <JobsTable filters={filters} />
      </div>
    </>
  );
}
function Diagnostics({ failure }: { failure?: Job["failure"] }) {
  return failure ? (
    <div className="diagnostics">
      <strong>
        {failure.code} {failure.exceptionType && `· ${failure.exceptionType}`}
      </strong>
      <p>{failure.message}</p>
      {failure.phase && <small>Phase: {failure.phase}</small>}
      {failure.stackTrace && <pre>{failure.stackTrace}</pre>}
      {failure.stackTraceTruncated && (
        <p className="warning">Stack trace truncated by the worker.</p>
      )}
    </div>
  ) : null;
}
function DatasetParts({
  jobId,
  location,
  expired,
}: {
  jobId: string;
  location: string;
  expired: boolean;
}) {
  const q = usePage<Part>(`/v1/admin/jobs/${jobId}/parts`, {}, false);
  return (
    <>
      <ErrorMessage error={q.error} />
      <table>
        <thead>
          <tr>
            <th>Published part</th>
            <th>Size</th>
            <th>Checksum</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {q.data?.items.map((p) => (
            <tr key={p.path}>
              <td>
                {p.path}
                <small className="mono">
                  {location}/{p.path}
                </small>
              </td>
              <td>{bytes(p.size)}</td>
              <td className="mono">{p.sha256}</td>
              <td>
                {!expired && (
                  <a
                    href={`/v1/admin/jobs/${jobId}/part?path=${encodeURIComponent(p.path)}`}
                  >
                    Download
                  </a>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <Pagination {...q} />
    </>
  );
}
function Results({ job }: { job: Job }) {
  const q = useData<Result>(`/v1/admin/jobs/${job.id}/result`, false);
  const [showPreview, setShowPreview] = useState(false);
  const preview = useData<Preview>(
    `/v1/admin/jobs/${job.id}/preview`,
    false,
    showPreview,
  );
  const result = q.data;
  return (
    <Card title="Published result">
      <ErrorMessage error={q.error} />
      {result && (
        <>
          <p>
            <State value={result.expired ? "EXPIRED" : "RETAINED"} /> ·
            Retention ends {date(result.expiresAt)} · {result.descriptor.kind}
          </p>
          <table>
            <thead>
              <tr>
                <th>File / location</th>
                <th>Size / type</th>
                <th>Checksum</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {result.files.map(({ file, location, unavailable }) => (
                <tr key={file.allocationId}>
                  <td>
                    {file.name}
                    <small className="mono">{location || unavailable}</small>
                  </td>
                  <td>
                    {bytes(file.size)}
                    <small>{file.contentType}</small>
                  </td>
                  <td className="mono">{file.sha256}</td>
                  <td>
                    {!result.expired && (
                      <a
                        href={`/v1/admin/jobs/${job.id}/files/${file.allocationId}`}
                      >
                        Download
                      </a>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="muted">
            Published paths are metadata. Your workstation needs its own access
            to the shared filesystem or bucket.
          </p>
          {result.descriptor.kind === "json" && !result.expired && (
            <>
              <Button variant="outline" onClick={() => setShowPreview(true)}>
                Preview JSON (64 KiB)
              </Button>
              <ErrorMessage error={preview.error} />
              {preview.data && (
                <>
                  <p className="muted">
                    {preview.data.truncated
                      ? "Preview truncated."
                      : preview.data.validJSON
                        ? "Valid JSON."
                        : "Result content is not valid JSON."}
                  </p>
                  <pre className="json">{preview.data.text}</pre>
                </>
              )}
            </>
          )}
          {result.dataset && (
            <>
              <h3>{result.dataset.metadata.format} dataset</h3>
              <p>
                {result.dataset.metadata.partCount} parts ·{" "}
                {bytes(result.dataset.metadata.totalBytes)}
              </p>
              <p className="mono">{result.dataset.location}</p>
              {!result.expired && (
                <DatasetParts
                  jobId={job.id}
                  location={result.dataset.location}
                  expired={result.expired}
                />
              )}
            </>
          )}
        </>
      )}
    </Card>
  );
}
export function RequestDetail({ operator }: { operator: boolean }) {
  const { id } = useParams();
  const queryClient = useQueryClient();
  const q = useData<Job>(
    `/v1/admin/jobs/${id}`,
    (data) => !data || !terminal(data.state),
  );
  const attempts = useData<Page<Attempt>>(
    `/v1/admin/jobs/${id}/attempts`,
    !q.data || !terminal(q.data.state),
  );
  const audits = usePage<Audit>(
    `/v1/admin/jobs/${id}/audit`,
    {},
    !q.data || !terminal(q.data.state),
  );
  const cancel = useMutation({
    mutationFn: () =>
      api<Job>(`/v1/admin/jobs/${id}/cancel`, { method: "POST", body: "{}" }),
    onSuccess: () => queryClient.invalidateQueries(),
  });
  const j = q.data;
  return (
    <>
      <Heading
        eyebrow="Request"
        title={id ?? "Request"}
        description={j ? `${j.contextName} · ${j.request.handler}` : undefined}
        action={
          operator && (
            <div className="actions">
              <Button variant="outline" asChild>
                <Link to={`/requests/new?sourceJobId=${id}`}>
                  Edit and replay
                </Link>
              </Button>
              {j && !terminal(j.state) && (
                <Button
                  variant="destructive"
                  disabled={cancel.isPending || j.state === "CANCELLING"}
                  onClick={() => {
                    if (
                      window.confirm(
                        "Cancel this request? Other jobs on its driver continue.",
                      )
                    )
                      cancel.mutate();
                  }}
                >
                  {j.state === "CANCELLING" ? "Cancelling…" : "Cancel request"}
                </Button>
              )}
            </div>
          )
        }
      />
      <ErrorMessage error={q.error} />
      <ErrorMessage error={cancel.error} />
      {j && (
        <>
          <Card title="Execution">
            <dl className="details">
              <Detail label="State">
                <State value={j.state} />
                {j.state === "CANCELLING" && (
                  <small>
                    Waiting for worker termination or lease recovery.
                  </small>
                )}
              </Detail>
              <Detail label="Context">
                <Link to={`/contexts/${j.contextId}`}>
                  {j.contextName} · {short(j.contextVersion)}
                </Link>
              </Detail>
              <Detail label="Owner">{j.owner}</Detail>
              <Detail label="Submitted / updated">
                {date(j.submittedAt)} / {date(j.updatedAt)}
              </Detail>
              <Detail label="Actor">{j.submittedBy || "SDK / legacy"}</Detail>
              {j.replayedFrom && (
                <Detail label="Replayed from">
                  <JobLink id={j.replayedFrom} />
                </Detail>
              )}
            </dl>
            {j.progress && (
              <div className="progress">
                <progress max={1} value={j.progress.fraction} />
                <span>
                  {Math.round(j.progress.fraction * 100)}% ·{" "}
                  {j.progress.message}
                </span>
              </div>
            )}
            <Diagnostics failure={j.failure} />
          </Card>
          <Card title="Input">
            <Json value={j.request} />
          </Card>
          <Card title="Attempts">
            <ErrorMessage error={attempts.error} />
            {attempts.data?.items.map((a) => (
              <details className="instance" key={a.id} open={!!a.failure}>
                <summary>
                  <strong>Attempt {a.number}</strong>
                  <State value={a.state} />
                  <span>
                    {date(a.startedAt)} →{" "}
                    {a.finishedAt ? date(a.finishedAt) : "In progress"}
                  </span>
                </summary>
                <p className="mono">
                  Worker {a.workerId} · {a.id}
                </p>
                <Diagnostics failure={a.failure} />
              </details>
            ))}
            {!attempts.data?.items.length && (
              <Empty>No attempt assigned yet.</Empty>
            )}
          </Card>
          <Card title="Administrative audit">
            <ErrorMessage error={audits.error} />
            {audits.data?.items.map((a) => (
              <div className="audit" key={a.id}>
                <strong>{a.action}</strong>
                <span>{a.actor}</span>
                <span>{date(a.acceptedAt)}</span>
                <State value={a.outcome} />
              </div>
            ))}
            {!audits.data?.items.length && (
              <Empty>No accepted administrative operations.</Empty>
            )}
            <Pagination {...audits} />
          </Card>
          {j.result && <Results job={j} />}
        </>
      )}
    </>
  );
}
export function NewRequest({ session }: { session: Session }) {
  const navigate = useNavigate();
  const [search] = useSearchParams();
  const sourceId = search.get("sourceJobId");
  const source = useData<Job>(`/v1/admin/jobs/${sourceId}`, false, !!sourceId);
  const [contextId, setContextId] = useState(search.get("contextId") ?? "");
  const [handler, setHandler] = useState("");
  const [version, setVersion] = useState(1);
  const [payload, setPayload] = useState("{}"),
    [attempts, setAttempts] = useState(1),
    [backoff, setBackoff] = useState(1),
    [deadline, setDeadline] = useState(""),
    [activate, setActivate] = useState(false);
  const [formError, setFormError] = useState("");
  const pending = useRef<{ fingerprint: string; key: string } | undefined>(
    undefined,
  );
  const initialized = useRef(false);
  useEffect(() => {
    if (source.data && !initialized.current) {
      const j = source.data;
      setContextId(j.contextId);
      setHandler(j.request.handler);
      setVersion(j.request.version);
      setPayload(JSON.stringify(j.request.payload, null, 2));
      setAttempts(j.request.retry.maxAttempts);
      setBackoff(j.request.retry.backoffSeconds);
      initialized.current = true;
    }
  }, [source.data]);
  const filters: Record<string, string> = { includeStopped: "true" };
  if (source.data) {
    filters.owner = source.data.owner;
    filters.name = source.data.contextName;
  }
  const contexts = usePage<Context>("/v1/admin/contexts", filters, false);
  const target = useData<Context>(
    `/v1/admin/contexts/${contextId}`,
    false,
    !!contextId,
  );
  const workers = useData<Page<Worker>>(
    `/v1/admin/contexts/${contextId}/workers`,
    false,
    !!contextId,
  );
  const submit = useMutation({
    mutationFn: (body: unknown) =>
      api<Job>(`/v1/admin/contexts/${contextId}/jobs`, {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: (j) => navigate(`/requests/${j.id}`),
    onError: (error) => {
      if (error instanceof APIError && error.status === 409) {
        void target.refetch();
        void contexts.refetch();
      }
    },
  });
  if (session.role !== "operator")
    return (
      <Empty>Operator access is required to create or replay requests.</Empty>
    );
  return (
    <>
      <Heading
        eyebrow={sourceId ? "Manual replay" : "Execution"}
        title={sourceId ? "Edit and replay" : "New request"}
        description="Accept a durable request on an existing context version."
      />
      <ErrorMessage
        error={source.error ?? contexts.error ?? target.error ?? submit.error}
      />
      <form
        className="card request-form"
        onSubmit={(e) => {
          e.preventDefault();
          setFormError("");
          try {
            if (!contextId) throw new Error("Select a context version");
            const body = {
              handler,
              version,
              payload: decodeJSON<unknown>(payload),
              retry: { maxAttempts: attempts, backoffSeconds: backoff },
              ...(deadline
                ? { deadline: new Date(deadline).toISOString() }
                : {}),
              ...(sourceId ? { replayedFrom: sourceId } : {}),
              activateIfStopped: activate,
            };
            const fingerprint = JSON.stringify({ contextId, body });
            if (pending.current?.fingerprint !== fingerprint)
              pending.current = { fingerprint, key: newSubmissionKey() };
            submit.mutate({ ...body, idempotencyKey: pending.current.key });
          } catch (err) {
            setFormError(err instanceof Error ? err.message : "Invalid form");
          }
        }}
      >
        <label>
          Context version
          <select
            required
            value={contextId}
            onChange={(e) => {
              setContextId(e.target.value);
              setActivate(false);
            }}
          >
            <option value="">Select a context…</option>
            {target.data &&
              !contexts.data?.items.some((c) => c.id === contextId) && (
                <option value={contextId}>
                  {target.data.name} · {short(target.data.version)} ·{" "}
                  {target.data.state}
                </option>
              )}
            {contexts.data?.items.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name} · {short(c.version)} · {c.state} · {c.owner}
              </option>
            ))}
          </select>
        </label>
        <Pagination {...contexts} />
        {source.data && (
          <p className="muted">
            Source <JobLink id={source.data.id} />. Select another version of
            this owner's logical context explicitly. This creates another
            execution; business side effects may repeat.
          </p>
        )}
        {target.data && (
          <>
            <dl className="details">
              <Detail label="Selected version">{target.data.version}</Detail>
              <Detail label="Owner">{target.data.owner}</Detail>
              <Detail label="Image">{target.data.spec.image}</Detail>
            </dl>
            <details>
              <summary>Engine and result configuration</summary>
              <Json value={target.data.spec} />
            </details>
            {["STOPPED", "DRAINING"].includes(target.data.state) && (
              <label className="checkbox warning">
                <input
                  type="checkbox"
                  checked={activate}
                  onChange={(e) => setActivate(e.target.checked)}
                />
                Activate this stopped/draining version to finish the accepted
                request.
              </label>
            )}
          </>
        )}
        <div className="form-grid">
          <label>
            Handler
            <input
              list="handlers"
              required
              value={handler}
              onChange={(e) => setHandler(e.target.value)}
            />
            <datalist id="handlers">
              {[
                ...new Set(
                  workers.data?.items.flatMap((w) =>
                    w.capabilities.map((c) => c.handler),
                  ),
                ),
              ].map((h) => (
                <option key={h} value={h} />
              ))}
            </datalist>
          </label>
          <label>
            Contract version
            <input
              type="number"
              min={1}
              required
              value={version}
              onChange={(e) => setVersion(e.target.valueAsNumber)}
            />
          </label>
        </div>
        <label>
          JSON payload
          <textarea
            spellCheck={false}
            rows={15}
            required
            value={payload}
            onChange={(e) => setPayload(e.target.value)}
          />
        </label>
        <div className="form-grid">
          <label>
            Maximum attempts
            <input
              type="number"
              min={1}
              max={10}
              required
              value={attempts}
              onChange={(e) => setAttempts(e.target.valueAsNumber)}
            />
          </label>
          <label>
            Retry backoff (seconds)
            <input
              type="number"
              min={1}
              max={3600}
              required
              value={backoff}
              onChange={(e) => setBackoff(e.target.valueAsNumber)}
            />
          </label>
          <label>
            New deadline (optional)
            <input
              type="datetime-local"
              value={deadline}
              onChange={(e) => setDeadline(e.target.value)}
            />
          </label>
        </div>
        {formError && (
          <div role="alert" className="error">
            {formError}
          </div>
        )}
        <p className="muted">
          Accepted work survives browser closure and server restarts. A version
          with no live clients stops once its work finishes.
        </p>
        <Button
          type="submit"
          disabled={submit.isPending || (!!sourceId && !source.data)}
        >
          {submit.isPending
            ? "Accepting…"
            : sourceId
              ? "Submit replay"
              : "Submit request"}
        </Button>
      </form>
    </>
  );
}
export function Servers() {
  const q = usePage<Server>("/v1/admin/servers");
  return (
    <>
      <Heading
        eyebrow="Platform"
        title="Servers"
        description="Heartbeat records are informational. They do not elect a scheduler."
      />
      <ErrorMessage error={q.error} />
      <div className="card">
        <table>
          <thead>
            <tr>
              <th>Server</th>
              <th>State</th>
              <th>Version</th>
              <th>Started</th>
              <th>Last heartbeat</th>
              <th>Dependencies</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.items.map((s) => (
              <tr key={s.id}>
                <td>
                  {s.pod || short(s.id)}
                  <small className="mono">{s.id}</small>
                </td>
                <td>
                  <State value={s.state} />
                </td>
                <td>{s.version}</td>
                <td>{date(s.startedAt)}</td>
                <td>{date(s.heartbeatAt)}</td>
                <td>
                  Database {s.detail.database ? "available" : "unavailable"}
                  <small>
                    Storage{" "}
                    {s.detail.localStorage
                      ? "available"
                      : "unavailable / not configured"}
                  </small>
                  <details>
                    <summary>Process observations</summary>
                    <Json value={s.detail} />
                  </details>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!q.data?.items.length && (
          <Empty>No recent server heartbeat records.</Empty>
        )}
        <Pagination {...q} />
      </div>
      <p className="muted">
        Heartbeats older than 30 seconds are stale. Records expire after 24
        hours.
      </p>
    </>
  );
}
