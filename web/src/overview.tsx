import { Link } from "react-router-dom";
import { useData } from "./api";
import type { OverviewData } from "./types";
import {
  Card,
  date,
  Empty,
  ErrorMessage,
  Heading,
  JobLink,
  State,
} from "./components/common";
import { Button } from "./components/ui/button";

function duration(value: number | null) {
  if (value === null) return "No completed executions";
  if (value < 60)
    return `${value.toLocaleString(undefined, { maximumFractionDigits: 1 })} s`;
  if (value < 3600) return `${(value / 60).toFixed(1)} min`;
  return `${(value / 3600).toFixed(1)} h`;
}
function Metric({
  label,
  value,
  note,
  tone = "",
}: {
  label: string;
  value: number | string;
  note: string;
  tone?: string;
}) {
  return (
    <div className={`stat overview-stat ${tone}`}>
      <span>{label}</span>
      <strong>
        {typeof value === "number" ? value.toLocaleString() : value}
      </strong>
      <small>{note}</small>
    </div>
  );
}
export function Overview({ operator }: { operator: boolean }) {
  const query = useData<OverviewData>("/v1/admin/overview");
  const data = query.data;
  return (
    <>
      <Heading
        eyebrow="Operations at a glance"
        title="Overview"
        description="Current activity, recent outcomes and the capacity serving your requests."
        action={
          operator && (
            <Button asChild>
              <Link to="/requests/new">New request</Link>
            </Button>
          )
        }
      />
      <ErrorMessage error={query.error} />
      {!data && query.isPending && <Empty>Loading overview…</Empty>}
      {data && (
        <>
          <div className="overview-window">
            <span>Current snapshot · {date(data.observedAt)}</span>
            <span>Recent outcomes · last 24 hours</span>
          </div>
          <div className="stats">
            <Metric
              label="Running now"
              value={data.runningJobs}
              note={`${data.cancellingJobs} cancelling`}
              tone="accent"
            />
            <Metric
              label="Queued now"
              value={data.queuedJobs}
              note={`${data.retryingJobs} waiting to retry`}
            />
            <Metric
              label="Failed · last 24h"
              value={data.failedJobs24h}
              note="Final request failures, including after retries"
              tone={data.failedJobs24h ? "danger" : ""}
            />
            <Metric
              label="Succeeded · last 24h"
              value={data.succeededJobs24h}
              note={`${data.cancelledJobs24h} cancelled in the same period`}
            />
          </div>
          <div className="stats overview-secondary">
            <Metric
              label="Active context versions"
              value={data.activeContexts}
              note="Starting, ready or draining"
            />
            <Metric
              label="Client connections"
              value={data.liveClients}
              note="Live connections to context versions"
            />
            <Metric
              label="Execution slots in use"
              value={`${data.occupiedSlots} / ${data.workerSlots}`}
              note={`${data.readyWorkers} ready workers`}
            />
            <Metric
              label="Servers reporting ready"
              value={data.serversReady}
              note={`${data.serversUnready} unready · ${data.serversStale} stale heartbeats`}
              tone={data.serversUnready || data.serversStale ? "attention" : ""}
            />
          </div>
          <div className="overview-timing">
            <div>
              <span>Average processing time · last 24h</span>
              <strong>{duration(data.meanProcessingSeconds24h)}</strong>
              <small>
                Finished executions; queue wait excluded, retries counted
                separately.
              </small>
            </div>
            <div>
              <span>Oldest queued request</span>
              <strong>
                {data.queuedJobs
                  ? duration(data.oldestQueuedSeconds)
                  : "Queue empty"}
              </strong>
              <small>
                Time since submission, including requests waiting to retry.
              </small>
            </div>
          </div>
          <div className="overview-grid">
            <Card title="Active contexts">
              <p className="muted">
                Latest active versions; provisioning issues appear first.
              </p>
              {data.activeContextDetails.length ? (
                data.activeContextDetails.map((c) => (
                  <article className="overview-item" key={c.id}>
                    <div className="overview-item-heading">
                      <Link to={`/contexts/${c.id}`}>{c.name}</Link>
                      <State value={c.state} />
                    </div>
                    <small>
                      {c.owner} · {c.runningJobs} running or cancelling ·{" "}
                      {c.queuedJobs} queued
                    </small>
                    {c.condition && <p className="warning">{c.condition}</p>}
                  </article>
                ))
              ) : (
                <Empty>No active context versions.</Empty>
              )}
              <Link className="section-link" to="/contexts">
                Browse all contexts →
              </Link>
            </Card>
            <Card title="Recent failures">
              <p className="muted">
                Latest five failures since {date(data.since)}.
              </p>
              {data.recentFailures.length ? (
                data.recentFailures.map((j) => (
                  <article className="overview-item" key={j.id}>
                    <div className="overview-item-heading">
                      <JobLink id={j.id} />
                      <small>{date(j.updatedAt)}</small>
                    </div>
                    <small>
                      {j.contextName} · {j.handler} · {j.owner}
                    </small>
                    <p className="failure-message">{j.message}</p>
                  </article>
                ))
              ) : (
                <Empty>No failed requests in the last 24 hours.</Empty>
              )}
              <Link className="section-link" to="/requests?state=FAILED">
                Browse all failed requests →
              </Link>
            </Card>
          </div>
          <div className="overview-links">
            <Link to="/requests">All requests</Link>
            <Link to="/servers">Server status and heartbeats</Link>
          </div>
        </>
      )}
    </>
  );
}
