import { useState } from "react";
import { Link } from "react-router";

import type { Channel } from "../api/client";
import { useMetrics } from "../api/queries";
import { modeOf, modeSettings, useProviderStats, useUpdateProvider, type ProviderMode, type ProviderSettings } from "../api/provider";
import { Button } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { inputClass } from "../components/inputClass";
import { ProblemNotice } from "../components/ProblemNotice";
import { SegmentedControl } from "../components/SegmentedControl";
import { Skeleton } from "../components/Skeleton";
import { Lamp } from "../components/StatusMark";
import { useToast } from "../components/Toasts";
import { CHANNELS, channelInfo } from "../lib/channels";
import { formatCount, shortId, timecode } from "../lib/format";
import type { Tone } from "../lib/status";
import styles from "./ProviderPage.module.css";

const modes: { value: ProviderMode; label: string }[] = [
  { value: "stable", label: "Stable" },
  { value: "flaky", label: "Flaky" },
  { value: "outage", label: "Outage" },
];

function toneOf(code: number): Tone {
  if (code >= 200 && code < 300) return "clear";
  if (code >= 500) return "probe";
  return "alarm";
}

export function ProviderPage() {
  const stats = useProviderStats(1000);
  const metrics = useMetrics("1m");
  const ceiling = metrics.data?.channels[0]?.rate_limit_per_second ?? 100;

  if (stats.isError) {
    return (
      <div className={styles.page}>
        <h1 className={styles.title}>Provider</h1>
        <EmptyState title="Deliveries go to an external provider">
          The bundled provider simulator is not reachable, which means <code>PROVIDER_URL</code> points somewhere else, such
          as webhook.site. Inspect deliveries there; the rest of the console works as usual.
        </EmptyState>
      </div>
    );
  }

  const s = stats.data;
  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.title}>Provider</h1>
        <p className={styles.sub}>
          The bundled simulator standing in for webhook.site. Break it on purpose to watch retries and circuit breaking
          on the <Link to="/">Overview</Link>.
        </p>
      </header>

      <div className={styles.grid}>
        <section className={styles.panel} aria-labelledby="behaviour-title">
          <h2 id="behaviour-title" className={styles.panelTitle}>
            Behaviour
          </h2>
          {s ? <Behaviour settings={s.settings} /> : <Skeleton height={180} />}
        </section>

        <section className={styles.panel} aria-labelledby="observed-title">
          <h2 id="observed-title" className={styles.panelTitle}>
            What the provider received
          </h2>
          <p className={styles.note}>
            Counted by the provider itself, independently of the system’s own metrics. Workers start at most {ceiling} sends
            per second per channel; network and provider latency can bunch arrivals by a few within a single second.
          </p>
          <ul className={styles.rates}>
            {CHANNELS.map((c: Channel) => {
              const ch = s?.channels[c];
              return (
                <li key={c}>
                  <span className={styles.rateName}>{channelInfo[c].label}</span>
                  <span className={styles.track} aria-hidden="true">
                    <span className={styles.peak} style={{ "--share": Math.min(1, (ch?.peak_per_second ?? 0) / ceiling) } as React.CSSProperties} />
                    <span className={styles.now} style={{ "--share": Math.min(1, (ch?.last_second ?? 0) / ceiling) } as React.CSSProperties} />
                  </span>
                  <span className={`${styles.rateNums} data`}>
                    {ch?.last_second ?? 0}/s now · peak {ch?.peak_per_second ?? 0}/s
                  </span>
                </li>
              );
            })}
          </ul>
          {s && (
            <dl className={styles.totals}>
              <div>
                <dt>Received</dt>
                <dd className="data">{formatCount(s.totals.received)}</dd>
              </div>
              <div>
                <dt>Accepted (202)</dt>
                <dd className="data">{formatCount(s.totals.accepted)}</dd>
              </div>
              <div>
                <dt>Failed (503)</dt>
                <dd className="data">{formatCount(s.totals.failed)}</dd>
              </div>
              <div>
                <dt>Rejected (4xx)</dt>
                <dd className="data">{formatCount(s.totals.rejected)}</dd>
              </div>
            </dl>
          )}
        </section>
      </div>

      <section className={styles.panel} aria-labelledby="recent-title">
        <h2 id="recent-title" className={styles.panelTitle}>
          Latest requests
        </h2>
        {s && s.recent.length === 0 ? (
          <p className={styles.note}>No request has reached the provider yet.</p>
        ) : (
          <div className={styles.scroller}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th scope="col">Received</th>
                  <th scope="col">Channel</th>
                  <th scope="col">To</th>
                  <th scope="col">Answer</th>
                  <th scope="col">Notification</th>
                </tr>
              </thead>
              <tbody>
                {s?.recent.map((m, i) => (
                  <tr key={`${m.received_at}-${i}`}>
                    <td className="data">{timecode(m.received_at)}</td>
                    <td>{channelInfo[m.channel]?.label ?? m.channel}</td>
                    <td className="data">{m.to}</td>
                    <td>
                      <Lamp tone={toneOf(m.status)} label={String(m.status)} />
                    </td>
                    <td className="data">
                      {m.idempotency_key ? <Link to={`/notifications?id=${m.idempotency_key}`}>{shortId(m.idempotency_key)}</Link> : "–"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

function Behaviour({ settings }: { settings: ProviderSettings }) {
  const update = useUpdateProvider();
  const toast = useToast();
  // Unsaved edits sit on top of the live settings, so the once-a-second
  // refresh never overwrites what the operator is typing.
  const [edits, setEdits] = useState<Partial<ProviderSettings>>({});
  const draft = { ...settings, ...edits };
  const setDraft = (next: ProviderSettings) => setEdits(next);

  const apply = (next: ProviderSettings) =>
    update.mutate(next, {
      onSuccess: () => {
        setEdits({});
        toast({ tone: "neutral", title: "Provider behaviour updated" });
      },
    });

  return (
    <div className={styles.behaviour}>
      <Field label="Mode" hint="Stable fails 3% of sends, flaky 35%; an outage fails every send.">
        {() => (
          <SegmentedControl
            label="Mode"
            value={modeOf(settings)}
            options={modes.map((m) => ({ ...m, disabled: update.isPending }))}
            onChange={(m) => apply({ ...settings, ...modeSettings[m] })}
          />
        )}
      </Field>
      <Field label="Failure rate" aside={`${Math.round(draft.failure_rate * 100)}%`} hint="Share of sends answered 503 and retried with backoff.">
        {(a) => (
          <input
            {...a}
            type="range"
            min={0}
            max={1}
            step={0.01}
            className={styles.range}
            value={draft.failure_rate}
            onChange={(e) => setDraft({ ...draft, failure_rate: Number(e.target.value) })}
          />
        )}
      </Field>
      <div className={styles.latency}>
        <Field label="Latency from (ms)">
          {(a) => <input {...a} type="number" min={0} className={inputClass} value={draft.min_latency_ms} onChange={(e) => setDraft({ ...draft, min_latency_ms: Number(e.target.value) })} />}
        </Field>
        <Field label="to (ms)">
          {(a) => <input {...a} type="number" min={0} className={inputClass} value={draft.max_latency_ms} onChange={(e) => setDraft({ ...draft, max_latency_ms: Number(e.target.value) })} />}
        </Field>
      </div>
      {update.isError && <ProblemNotice error={update.error} />}
      <div className={styles.apply}>
        <Button busy={update.isPending} onClick={() => apply(draft)} disabled={Object.keys(edits).length === 0}>
          Apply
        </Button>
      </div>
    </div>
  );
}
