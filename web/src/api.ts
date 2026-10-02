import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { Page } from "./types";
export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
let csrf = "";
export function setCSRF(value?: string) {
  csrf = value ?? "";
}
export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(csrf ? { "X-Linha-CSRF": csrf } : {}),
      ...options?.headers,
    },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new APIError(
      response.status,
      body.error?.code ?? "UNAVAILABLE",
      body.error?.message ?? `Request failed (${response.status})`,
    );
  }
  if (response.status === 204) return undefined as T;
  return decodeJSON<T>(await response.text());
}
export function useVisible() {
  const [visible, setVisible] = useState(!document.hidden);
  useEffect(() => {
    const listener = () => setVisible(!document.hidden);
    document.addEventListener("visibilitychange", listener);
    return () => document.removeEventListener("visibilitychange", listener);
  }, []);
  return visible;
}
export function useData<T>(
  path: string,
  active: boolean | ((data: T | undefined) => boolean) = true,
  enabled = true,
) {
  const visible = useVisible();
  return useQuery({
    queryKey: [path],
    queryFn: () => api<T>(path),
    enabled: enabled && visible,
    refetchInterval: (query) =>
      visible &&
      (typeof active === "function" ? active(query.state.data) : active) &&
      !(query.state.error instanceof APIError && query.state.error.status < 500)
        ? Math.min(
            60_000,
            5000 * 2 ** Math.min(query.state.fetchFailureCount, 4),
          )
        : false,
    refetchIntervalInBackground: false,
    retry: (count, error) =>
      !(error instanceof APIError && error.status < 500) && count < 2,
  });
}
export function usePage<T>(
  path: string,
  filters: Record<string, string> = {},
  active = true,
  enabled = true,
) {
  const [history, setHistory] = useState<string[]>([""]);
  const identity = JSON.stringify(filters);
  useEffect(() => setHistory([""]), [identity, path]);
  const params = new URLSearchParams(filters);
  params.set("limit", "50");
  if (history.at(-1)) params.set("cursor", history.at(-1)!);
  const query = useData<Page<T>>(`${path}?${params}`, active, enabled);
  return {
    ...query,
    next: () =>
      query.data?.nextCursor && setHistory([...history, query.data.nextCursor]),
    previous: () => setHistory(history.slice(0, -1)),
    hasPrevious: history.length > 1,
  };
}

// Keep JVM-sized integers intact when a job envelope is displayed and replayed.
export function decodeJSON<T>(text: string): T {
  return JSON.parse(
    text,
    (_key, value: unknown, context?: { source: string }) => {
      if (
        typeof value === "number" &&
        ((Number.isInteger(value) && !Number.isSafeInteger(value)) ||
          !Number.isFinite(value))
      ) {
        const raw = (
          JSON as unknown as { rawJSON?: (source: string) => unknown }
        ).rawJSON;
        if (!raw || !context?.source)
          throw new Error(
            "This browser cannot preserve large JSON integers. Use a current browser to inspect or replay this request.",
          );
        return raw(context.source);
      }
      return value;
    },
  ) as T;
}

export function newSubmissionKey(): string {
  if (crypto.randomUUID) return crypto.randomUUID();
  return [...crypto.getRandomValues(new Uint8Array(16))]
    .map((value) => value.toString(16).padStart(2, "0"))
    .join("");
}
