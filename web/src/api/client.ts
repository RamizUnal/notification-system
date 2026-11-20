import createClient from "openapi-fetch";

import type { components, paths } from "./schema";

// Wire types generated from api/openapi.yaml (npm run generate:api), so the
// console and the server share one contract.
export type Notification = components["schemas"]["Notification"];
export type NotificationInput = components["schemas"]["NotificationInput"];
export type NotificationPage = components["schemas"]["NotificationPage"];
export type Attempt = components["schemas"]["Attempt"];
export type Batch = components["schemas"]["Batch"];
export type BatchCreated = components["schemas"]["BatchCreated"];
export type Template = components["schemas"]["Template"];
export type TemplateInput = components["schemas"]["TemplateInput"];
export type Metrics = components["schemas"]["Metrics"];
export type ChannelMetrics = components["schemas"]["ChannelMetrics"];
export type Worker = components["schemas"]["Worker"];
export type Channel = components["schemas"]["Channel"];
export type Priority = components["schemas"]["Priority"];
export type Status = components["schemas"]["Status"];

/** An RFC 9457 problem as returned by the API (or synthesized for network failures). */
export interface Problem {
  type?: string;
  title: string;
  status: number;
  detail?: string;
  code: string;
  request_id?: string;
  errors?: { field: string; code: string; message: string }[];
  conflicts?: { index: number; idempotency_key: string; notification_id: string }[];
}

// fetch is looked up per request (not captured once), so tests can stub it.
export const api = createClient<paths>({
  baseUrl: typeof window === "undefined" ? "" : window.location.origin,
  fetch: (request: Request) => globalThis.fetch(request),
});

/** ApiError carries the problem details behind a failed request. */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem;

  constructor(problem: Problem) {
    super(problem.detail ?? problem.title);
    this.name = "ApiError";
    this.status = problem.status;
    this.problem = problem;
  }

  /** Field paths mapped to their messages, for inline form errors. */
  get fieldErrors(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const e of this.problem.errors ?? []) {
      out[e.field] ??= e.message;
    }
    return out;
  }
}

interface FetchResult<T> {
  data?: T;
  error?: unknown;
  response: Response;
}

/**
 * call awaits an openapi-fetch request and returns its data, or throws an
 * ApiError: the API's own problem details when it answered, a synthesized
 * one when it could not be reached at all.
 */
export async function call<T>(request: Promise<FetchResult<T>>): Promise<T> {
  let result: FetchResult<T>;
  try {
    result = await request;
  } catch {
    throw new ApiError({
      title: "Network error",
      status: 0,
      code: "network_error",
      detail: "The API could not be reached. Check that it is running.",
    });
  }
  if (result.error !== undefined || !result.response.ok) {
    throw toApiError(result.error, result.response);
  }
  return result.data as T;
}

export function toApiError(body: unknown, response: Response): ApiError {
  if (isProblem(body)) {
    return new ApiError(body);
  }
  return new ApiError({
    title: response.statusText || "Request failed",
    status: response.status,
    code: "unexpected_response",
    detail: `The server answered ${response.status} without problem details.`,
  });
}

function isProblem(value: unknown): value is Problem {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as Problem).status === "number" &&
    typeof (value as Problem).code === "string"
  );
}
