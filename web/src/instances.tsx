import { useState } from "react";
import { usePage } from "./api";
import type { Instance } from "./types";
import {
  bytes,
  date,
  Detail,
  Empty,
  ErrorMessage,
  Pagination,
  short,
  State,
} from "./components/common";

function Allocation({
  instance,
  historical,
}: {
  instance: Instance;
  historical: boolean;
}) {
  const resources = instance.detail?.resources;
  const current =
    !historical &&
    !instance.stale &&
    instance.available &&
    instance.state !== "DRAINING";
  const value = (role: string, resource: string, kind: string) => {
    const n = resources?.[`${role}_${resource}_${kind}`];
    return n === undefined
      ? "Not observed"
      : resource === "memory"
        ? bytes(n)
        : `${n.toLocaleString(undefined, { maximumFractionDigits: 3 })} cores`;
  };
  return (
    <section className={`allocation ${current ? "" : "allocation-history"}`}>
      <h4>
        {historical
          ? "Historical resource allocation"
          : current
            ? "Current resource allocation"
            : "Last known resource allocation"}
      </h4>
      <p className="muted">
        {historical
          ? "Retained snapshot; excluded from current capacity."
          : current
            ? "Configured requests and limits for this instance."
            : "This snapshot is not confirmed as current."}{" "}
        Observed {date(instance.observedAt)}.
      </p>
      {resources ? (
        <div className="allocation-table">
          <table>
            <thead>
              <tr>
                <th>Role</th>
                <th>CPU request</th>
                <th>CPU limit</th>
                <th>Memory request</th>
                <th>Memory limit</th>
              </tr>
            </thead>
            <tbody>
              {["driver", "executor"].map((role) => (
                <tr key={role}>
                  <td>{role === "driver" ? "Driver" : "Executors (total)"}</td>
                  <td>{value(role, "cpu", "request")}</td>
                  <td>{value(role, "cpu", "limit")}</td>
                  <td>{value(role, "memory", "request")}</td>
                  <td>{value(role, "memory", "limit")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="muted">Resource settings have not been observed.</p>
      )}
      <p className="muted">
        Requests reserve resources; limits cap them. These values do not measure
        CPU or memory usage.
      </p>
    </section>
  );
}
function InstanceCard({
  instance: i,
  engine,
  historical = false,
}: {
  instance: Instance;
  engine: string;
  historical?: boolean;
}) {
  const published =
    !historical &&
    i.state !== "DRAINING" &&
    !i.stale &&
    (i.available || i.detail?.condition === "missing_resources") &&
    i.detail?.links?.length;
  return (
    <details
      className={`engine-instance ${historical ? "retired-instance" : "current-instance"}`}
      open={!historical}
    >
      <summary>
        <div>
          <strong>{i.detail?.driverPod || `Instance ${short(i.id)}`}</strong>
          <small className="mono">
            {short(i.id)} ·{" "}
            {historical
              ? "Retired"
              : i.state === "DRAINING"
                ? "Stopping"
                : "Current instance"}
          </small>
        </div>
        <State value={i.state} />
        <span>
          {historical
            ? "Historical snapshot"
            : i.stale
              ? "Stale observation"
              : i.available
                ? "Observation available"
                : i.detail?.condition === "missing_resources"
                  ? "Allocation data incomplete"
                  : "Observation unavailable"}
        </span>
        <time>Last observed {date(i.observedAt)}</time>
      </summary>
      <div className="instance-body">
        {historical && (
          <p className="history-note">
            This instance has stopped. Pod states and resource values below
            describe its last recorded observation.
          </p>
        )}
        <dl className="details">
          <Detail label="Resource">
            {i.resourceKind} · {i.resourceUid || "Not observed"}
          </Detail>
          <Detail label="Incarnation">
            <span className="mono">{i.incarnation || "No driver pod"}</span>
          </Detail>
          {engine === "spark" && (
            <>
              <Detail
                label={
                  historical
                    ? "Last observed SparkApplication"
                    : "SparkApplication"
                }
              >
                {i.detail?.application ?? "Not observed"} ·{" "}
                {i.detail?.applicationState ?? "Unknown"}
              </Detail>
              <Detail
                label={historical ? "Last observed driver pod" : "Driver pod"}
              >
                {i.detail?.driverPod ?? "Not observed"} ·{" "}
                {i.detail?.driverPhase ?? "Unknown"}
              </Detail>
              <Detail
                label={
                  historical ? "Last observed executor pods" : "Executor pods"
                }
              >
                {Object.keys(i.detail?.executorStates ?? {}).length
                  ? Object.entries(i.detail!.executorStates!)
                      .map(([state, count]) => `${count} ${state}`)
                      .join(" · ")
                  : i.available && !i.stale && !historical
                    ? "0 executors"
                    : "Not observed"}
              </Detail>
              <Detail label="Spark UI">
                {published
                  ? i.detail!.links!.map((l) => (
                      <a
                        key={l.url}
                        href={l.url}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        Open {l.name}
                      </a>
                    ))
                  : "No current published Spark UI URL"}
                <small>
                  {i.detail?.uiService &&
                    `${i.detail.uiService}:${i.detail.uiPort}`}
                </small>
              </Detail>
            </>
          )}
        </dl>
        {i.detail?.condition && (
          <p className="warning">{i.detail.condition.replaceAll("_", " ")}</p>
        )}
        {(engine === "spark" || i.detail?.resources) && (
          <Allocation instance={i} historical={historical} />
        )}
      </div>
    </details>
  );
}
export function Instances({ id, engine }: { id: string; engine: string }) {
  const [historyOpen, setHistoryOpen] = useState(false);
  const current = usePage<Instance>(`/v1/admin/contexts/${id}/instances`, {
    state: "active",
  });
  const history = usePage<Instance>(
    `/v1/admin/contexts/${id}/instances`,
    { state: "DEAD" },
    historyOpen,
    historyOpen,
  );
  return (
    <>
      <section aria-label="Current instances" className="instance-section">
        <h3>Current instances</h3>
        <p className="muted">
          Starting, running and stopping instances. Retired instances are kept
          separately below.
        </p>
        <ErrorMessage error={current.error} />
        {current.data?.items
          .filter((i) => i.state !== "DEAD")
          .map((i) => (
            <InstanceCard key={i.id} instance={i} engine={engine} />
          ))}
        {current.isPending ? (
          <Empty>Loading current instances…</Empty>
        ) : (
          !current.error &&
          !current.data?.items.length && (
            <Empty>No current engine instances.</Empty>
          )
        )}
        <Pagination {...current} />
      </section>
      <details
        className="instance-history"
        onToggle={(e) => setHistoryOpen(e.currentTarget.open)}
      >
        <summary>
          Retired instances <span>Historical observations and allocations</span>
        </summary>
        {historyOpen && (
          <section aria-label="Retired instances">
            <ErrorMessage error={history.error} />
            {history.data?.items.map((i) => (
              <InstanceCard
                key={i.id}
                instance={i}
                engine={engine}
                historical
              />
            ))}
            {history.isPending ? (
              <Empty>Loading retired instances…</Empty>
            ) : (
              !history.error &&
              !history.data?.items.length && (
                <Empty>No retired engine instances.</Empty>
              )
            )}
            <Pagination {...history} />
          </section>
        )}
      </details>
    </>
  );
}
