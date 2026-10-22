import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, useContextChoices, usePage } from "./api";
import { Button } from "./components/ui/button";
import {
  date,
  Empty,
  ErrorMessage,
  Filters,
  Heading,
  JobLink,
  Pagination,
  short,
  State,
} from "./components/common";
import type { BatchResult, Context, Job } from "./types";

const jobStates = [
  "QUEUED",
  "RETRYING",
  "RUNNING",
  "CANCELLING",
  "SUCCEEDED",
  "FAILED",
  "CANCELLED",
];
const pending = (job: Job) => ["QUEUED", "RETRYING"].includes(job.state);

function compatible(target: Context, jobs: Job[], contexts: Context[]) {
  return (
    ["READY", "STARTING"].includes(target.state) &&
    jobs.every((job) => {
      const source = contexts.find((c) => c.id === job.contextId);
      return (
        source &&
        target.id !== source.id &&
        target.owner === source.owner &&
        target.name === source.name &&
        target.spec.engine.type === source.spec.engine.type &&
        target.spec.engine.version === source.spec.engine.version &&
        (source.spec.storage?.destinations ?? []).every(
          (name) =>
            target.spec.storage?.destinations.includes(name) &&
            source.spec.storage?.fingerprints?.[name] ===
              target.spec.storage?.fingerprints?.[name],
        )
      );
    })
  );
}

function BatchDialog({
  action,
  jobs,
  contexts,
  loading,
  error,
  busy,
  canMove,
  onClose,
  onConfirm,
}: {
  action: "move" | "cancel";
  jobs: Job[];
  contexts: Context[];
  loading: boolean;
  error: unknown;
  busy: boolean;
  canMove: boolean;
  onClose: () => void;
  onConfirm: (target?: string) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [search, setSearch] = useState("");
  const [target, setTarget] = useState("");
  useEffect(() => {
    const el = dialog.current;
    el?.showModal();
    return () => el?.close();
  }, []);
  const targets = contexts.filter((c) => compatible(c, jobs, contexts));
  const matching = targets.filter((c) =>
    `${c.name} ${c.version} ${c.id} ${c.state}`
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  return (
    <dialog
      ref={dialog}
      className="batch-dialog"
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
      aria-labelledby="batch-title"
    >
      <h2 id="batch-title">
        {action === "move" ? "Move" : "Cancel"} {jobs.length} selected{" "}
        {jobs.length === 1 ? "request" : "requests"}
      </h2>
      {action === "move" ? (
        <>
          <p>
            Request IDs, payloads, retry history and result storage stay the
            same. Confirm that the destination supports these handlers and
            contracts.
          </p>
          <label>
            Search destination
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Context name or version"
              disabled={busy}
            />
          </label>
          <label>
            Destination version
            <select
              value={target}
              onChange={(event) => setTarget(event.target.value)}
              disabled={busy || loading}
            >
              <option value="">Choose an active version</option>
              {matching.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} · {short(c.version)} · {c.state} · {c.readyWorkers}{" "}
                  ready workers
                </option>
              ))}
            </select>
          </label>
          {loading && <p>Loading context versions…</p>}
          {!loading && !targets.length && (
            <p className="warning">
              No compatible active destination is available. A destination must
              have the same owner, logical name, engine and source storage
              access.
            </p>
          )}
        </>
      ) : (
        <p>
          Queued requests stop immediately. Running requests receive a
          cancellation signal; their drivers remain available for other work.
        </p>
      )}
      <ErrorMessage error={error} />
      {action === "move" && !canMove && (
        <p className="warning">
          Some selected requests changed state or context. Go back and reselect
          them before moving.
        </p>
      )}
      <div className="batch-dialog-actions">
        <Button variant="outline" disabled={busy} onClick={onClose}>
          Back
        </Button>
        <Button
          variant={action === "cancel" ? "destructive" : "default"}
          disabled={
            busy ||
            (action === "move" &&
              (!canMove || !targets.some((c) => c.id === target)))
          }
          onClick={() => onConfirm(target)}
        >
          {busy
            ? "Applying…"
            : action === "move"
              ? "Confirm move"
              : "Confirm cancellation"}
        </Button>
      </div>
    </dialog>
  );
}

export function JobsTable({
  contextId,
  filters = {},
  operator,
  enabled = true,
}: {
  contextId?: string;
  filters?: Record<string, string>;
  operator: boolean;
  enabled?: boolean;
}) {
  const q = usePage<Job>(
    "/v1/admin/jobs",
    { ...filters, ...(contextId ? { contextId } : {}) },
    true,
    enabled,
  );
  const contexts = useContextChoices(filters.owner);
  const client = useQueryClient();
  const [selected, setSelected] = useState<Record<string, Job>>({});
  const [action, setAction] = useState<"move" | "cancel" | null>(null);
  const [result, setResult] = useState<BatchResult>();
  const scope = JSON.stringify({ contextId, filters });
  useEffect(() => {
    setSelected({});
    setAction(null);
    setResult(undefined);
  }, [scope]);
  const rows = q.data?.items ?? [];
  const visibleIDs = rows.map((job) => job.id).join(",");
  useEffect(() => {
    const ids = new Set(visibleIDs.split(","));
    setSelected((previous) =>
      Object.fromEntries(
        Object.entries(previous).filter(([id]) => ids.has(id)),
      ),
    );
  }, [visibleIDs]);

  const jobs = Object.values(selected);
  const canMove =
    jobs.length > 0 &&
    jobs.every((selected) => {
      const current = rows.find((job) => job.id === selected.id);
      return (
        current && current.contextId === selected.contextId && pending(current)
      );
    });
  const mutation = useMutation({
    mutationFn: ({
      action,
      target,
    }: {
      action: "move" | "cancel";
      target?: string;
    }) =>
      api<BatchResult>("/v1/admin/jobs/batch", {
        method: "POST",
        body: JSON.stringify({
          action,
          jobs: jobs.map((j) => ({ id: j.id, contextId: j.contextId })),
          ...(action === "move" ? { targetContextId: target } : {}),
        }),
      }),
    onSuccess: (value) => {
      setResult(value);
      setAction(null);
      setSelected((previous) =>
        Object.fromEntries(
          value.items
            .filter((item) => item.error && previous[item.id])
            .map((item) => [item.id, previous[item.id]]),
        ),
      );
      void client.invalidateQueries();
    },
  });
  const selectable = rows.filter(
    (job) => !["SUCCEEDED", "FAILED", "CANCELLED"].includes(job.state),
  );
  const selectedOnPage = selectable.filter((job) => selected[job.id]).length;
  const failed = result?.items.filter((item) => item.error).length ?? 0;
  if (!enabled)
    return <Empty>Choose a valid date range to load requests.</Empty>;
  return (
    <>
      <ErrorMessage error={q.error} />
      <ErrorMessage error={mutation.error} />
      {operator && (
        <div className="batch-toolbar">
          <span>
            <strong>{jobs.length} selected</strong>
            <small>Selection applies to this page only.</small>
          </span>
          <div>
            <Button
              size="small"
              variant="outline"
              disabled={!jobs.length || mutation.isPending}
              onClick={() => {
                mutation.reset();
                setAction("cancel");
              }}
            >
              Cancel selected
            </Button>
            <Button
              size="small"
              variant="outline"
              disabled={!jobs.length || !canMove || mutation.isPending}
              onClick={() => {
                mutation.reset();
                setAction("move");
              }}
            >
              Move selected
            </Button>
            <Button
              size="small"
              variant="outline"
              disabled={!jobs.length || mutation.isPending}
              onClick={() => setSelected({})}
            >
              Clear selection
            </Button>
          </div>
          {jobs.length > 0 && !canMove && (
            <small>
              Only queued or retrying requests can move. Reselect requests whose
              context changed.
            </small>
          )}
        </div>
      )}
      {result && (
        <section
          className="batch-results"
          aria-label="Batch results"
          role="status"
        >
          <strong>
            {result.items.length - failed} completed · {failed} need attention
          </strong>
          <ul>
            {result.items.map((item) => (
              <li key={item.id}>
                <JobLink id={item.id} /> · {item.outcome}
                {item.error && `: ${item.error.message}`}
              </li>
            ))}
          </ul>
        </section>
      )}
      <table>
        <thead>
          <tr>
            {operator && (
              <th className="selection-cell">
                <input
                  type="checkbox"
                  aria-label="Select all requests on this page"
                  disabled={!selectable.length || mutation.isPending}
                  checked={
                    !!selectable.length && selectedOnPage === selectable.length
                  }
                  ref={(el) => {
                    if (el)
                      el.indeterminate =
                        selectedOnPage > 0 &&
                        selectedOnPage < selectable.length;
                  }}
                  onChange={(event) =>
                    setSelected(
                      event.target.checked
                        ? Object.fromEntries(
                            selectable.map((job) => [job.id, job]),
                          )
                        : {},
                    )
                  }
                />
              </th>
            )}
            <th>Request</th>
            <th>Context</th>
            <th>Handler</th>
            <th>State / progress</th>
            <th>Attempts</th>
            <th>Submitted</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((j) => (
            <tr key={j.id}>
              {operator && (
                <td className="selection-cell">
                  <input
                    type="checkbox"
                    aria-label={`Select request ${short(j.id)}`}
                    checked={!!selected[j.id]}
                    disabled={!selectable.includes(j) || mutation.isPending}
                    onChange={(event) =>
                      setSelected((previous) => {
                        const next = { ...previous };
                        if (event.target.checked) next[j.id] = j;
                        else delete next[j.id];
                        return next;
                      })
                    }
                  />
                </td>
              )}
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
      {!q.isPending && !rows.length && (
        <Empty>No requests match these filters.</Empty>
      )}
      {q.isPending && <Empty>Loading requests…</Empty>}
      <Pagination
        {...q}
        next={() => {
          setSelected({});
          setResult(undefined);
          q.next();
        }}
        previous={() => {
          setSelected({});
          setResult(undefined);
          q.previous();
        }}
      />
      {action && (
        <BatchDialog
          action={action}
          jobs={jobs}
          contexts={contexts.data ?? []}
          loading={contexts.isPending}
          error={mutation.error ?? contexts.error}
          busy={mutation.isPending}
          canMove={canMove}
          onClose={() => setAction(null)}
          onConfirm={(target) => mutation.mutate({ action, target })}
        />
      )}
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
  const contexts = useContextChoices(filters.owner);
  const invalid = !!(
    filters.from &&
    filters.until &&
    new Date(filters.from) > new Date(filters.until)
  );
  const requestFilters = { ...filters };
  for (const key of ["from", "until"])
    if (filters[key])
      requestFilters[key] = new Date(filters[key]).toISOString();
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
        onChange={(next) =>
          setFilters({
            ...next,
            ...(next.name !== filters.name
              ? { contextId: "", metricContext: "" }
              : {}),
          })
        }
        fields={[
          { key: "owner", label: "Owner" },
          {
            key: "name",
            label: "Context name",
            suggestions: [
              ...new Set(contexts.data?.map((c) => c.name) ?? []),
            ].sort(),
          },
          { key: "state", label: "State", options: jobStates },
          { key: "from", label: "From", type: "datetime-local" },
          { key: "until", label: "Until", type: "datetime-local" },
        ]}
      />
      <ErrorMessage error={contexts.error} />
      <p className="muted">Dates and times use your local timezone.</p>
      {(filters.contextId || filters.metricContext) && (
        <p className="muted">
          Filtered to{" "}
          {filters.contextId
            ? `context version ${short(filters.contextId)}`
            : `context group ${filters.metricContext}`}{" "}
          <Button
            size="small"
            variant="outline"
            onClick={() =>
              setFilters({ ...filters, contextId: "", metricContext: "" })
            }
          >
            Clear context scope
          </Button>
        </p>
      )}
      {invalid && (
        <div role="alert" className="error">
          From must be before Until.
        </div>
      )}
      <div className="card table-card">
        <JobsTable
          filters={requestFilters}
          operator={operator}
          enabled={!invalid}
        />
      </div>
    </>
  );
}
