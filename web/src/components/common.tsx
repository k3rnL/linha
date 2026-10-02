import { Link } from "react-router-dom";
import type { ReactNode } from "react";
import { Button } from "./ui/button";
export const date = (value?: string) =>
  value ? new Date(value).toLocaleString() : "Not observed";
export const short = (value: string) => value.slice(0, 12);
export const bytes = (value: number) =>
  value < 1024
    ? `${value} B`
    : value < 1024 ** 2
      ? `${(value / 1024).toFixed(1)} KiB`
      : value < 1024 ** 3
        ? `${(value / 1024 ** 2).toFixed(1)} MiB`
        : `${(value / 1024 ** 3).toFixed(1)} GiB`;
export function State({ value }: { value: string }) {
  return (
    <span className={`state state-${value.toLowerCase()}`}>
      {value.replaceAll("_", " ")}
    </span>
  );
}
export function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <div role="alert" className="error">
      {error instanceof Error ? error.message : "Data unavailable"}. Previously
      loaded data may be stale.
    </div>
  ) : null;
}
export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}
export function Json({ value }: { value: unknown }) {
  return <pre className="json">{JSON.stringify(value, null, 2)}</pre>;
}
export function Heading({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <header className="page-heading">
      <div>
        <div className="eyebrow">{eyebrow}</div>
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {action}
    </header>
  );
}
export function Card({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="card">
      <h2>{title}</h2>
      {children}
    </section>
  );
}
export function Pagination({
  next,
  previous,
  hasPrevious,
  data,
}: {
  next: () => void;
  previous: () => void;
  hasPrevious: boolean;
  data?: { nextCursor?: string; items: unknown[] };
}) {
  return (
    <div className="pagination">
      <span>{data?.items.length ?? 0} records on this page</span>
      <div>
        <Button
          variant="outline"
          size="small"
          disabled={!hasPrevious}
          onClick={previous}
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="small"
          disabled={!data?.nextCursor}
          onClick={next}
        >
          Next
        </Button>
      </div>
    </div>
  );
}
export function JobLink({ id }: { id: string }) {
  return (
    <Link className="mono" to={`/requests/${id}`}>
      {short(id)}
    </Link>
  );
}
export function Detail({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="detail">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
export function Filters({
  values,
  onChange,
  fields,
  children,
}: {
  values: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  fields: { key: string; label: string; options?: string[]; type?: string }[];
  children?: ReactNode;
}) {
  return (
    <div className="filters">
      {fields.map((f) => (
        <label key={f.key}>
          {f.label}
          {f.options ? (
            <select
              value={values[f.key] ?? ""}
              onChange={(e) => onChange({ ...values, [f.key]: e.target.value })}
            >
              <option value="">All</option>
              {f.options.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          ) : (
            <input
              type={f.type ?? "text"}
              value={values[f.key] ?? ""}
              onChange={(e) => onChange({ ...values, [f.key]: e.target.value })}
              placeholder={`Filter ${f.label.toLowerCase()}`}
            />
          )}
        </label>
      ))}
      {children}
    </div>
  );
}
