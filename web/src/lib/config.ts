/** Where the Jaeger UI is reachable from the operator's browser. */
export const jaegerURL: string = import.meta.env.VITE_JAEGER_URL ?? "http://localhost:16686";

export function traceLink(traceId: string): string {
  return `${jaegerURL}/trace/${traceId}`;
}
