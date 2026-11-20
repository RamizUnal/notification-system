import { RotateCw } from "lucide-react";

import { ApiError } from "../api/client";
import { shortId } from "../lib/format";
import { Button } from "./Button";
import styles from "./ProblemNotice.module.css";

interface ProblemNoticeProps {
  error: unknown;
  title?: string;
  onRetry?: () => void;
  /** Field errors already shown next to their inputs are not repeated. */
  hideFields?: boolean;
}

// Human titles for the API's stable problem codes; the HTTP phrase is the fallback.
const titles: Record<string, string> = {
  validation_failed: "Some fields need attention",
  invalid_parameters: "Some filters are invalid",
  invalid_json: "The request could not be read",
  not_found: "Not found",
  not_cancellable: "Too late to cancel",
  not_retryable: "Only failed notifications can be retried",
  idempotency_conflict: "That idempotency key was already used",
  template_name_taken: "That template name is taken",
  network_error: "The API is unreachable",
  internal_error: "Something went wrong on the server",
};

/** Explains a failed request: what went wrong, what to do, and the request ID to quote. */
export function ProblemNotice({ error, title, onRetry, hideFields }: ProblemNoticeProps) {
  const problem = error instanceof ApiError ? error.problem : undefined;
  const message = problem?.detail ?? (error instanceof Error ? error.message : "Something went wrong.");
  return (
    <div className={styles.notice} role="alert">
      <div className={styles.text}>
        <p className={styles.title}>{title ?? (problem && (titles[problem.code] ?? problem.title)) ?? "Request failed"}</p>
        <p>{message}</p>
        {!hideFields && problem?.errors && problem.errors.length > 0 && (
          <ul className={styles.fields}>
            {problem.errors.map((e) => (
              <li key={`${e.field}-${e.code}`}>
                <code>{e.field}</code> {e.message}
              </li>
            ))}
          </ul>
        )}
        {problem?.conflicts && problem.conflicts.length > 0 && (
          <ul className={styles.fields}>
            {problem.conflicts.map((c) => (
              <li key={c.index}>
                Item {c.index + 1} reuses <code>{c.idempotency_key}</code> (notification <code>{shortId(c.notification_id)}</code>)
              </li>
            ))}
          </ul>
        )}
        {problem?.request_id && (
          <p className={styles.request}>
            Request ID <code>{problem.request_id}</code>
          </p>
        )}
      </div>
      {onRetry && (
        <Button size="sm" icon={<RotateCw />} onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}
