import { Link } from "react-router";

import { EmptyState } from "../components/EmptyState";

export function NotFoundPage() {
  return (
    <EmptyState title="There is no screen at this address" action={<Link to="/">Back to the overview</Link>}>
      The link may be mistyped, or the page may have moved.
    </EmptyState>
  );
}
