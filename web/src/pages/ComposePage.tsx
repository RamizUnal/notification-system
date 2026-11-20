import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router";
import { Send, Shuffle } from "lucide-react";

import { ApiError, type Channel, type NotificationInput, type Priority } from "../api/client";
import { useBatch, useCreateBatch, useCreateNotification, useNotification, usePreviewTemplate, useTemplates } from "../api/queries";
import { BatchProgress } from "../components/BatchProgress";
import { Button } from "../components/Button";
import { Chip } from "../components/Chip";
import { Field } from "../components/Field";
import { inputClass } from "../components/inputClass";
import { ProblemNotice } from "../components/ProblemNotice";
import { SegmentedControl } from "../components/SegmentedControl";
import { StatusMark } from "../components/StatusMark";
import { CHANNELS, channelInfo, PRIORITIES, priorityLabel } from "../lib/channels";
import { synthesize } from "../lib/demo";
import { formatCount, shortId } from "../lib/format";
import { smsSegments } from "../lib/sms";
import styles from "./ComposePage.module.css";

export function ComposePage() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "batch" ? "batch" : "single";
  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <div>
          <h1 className={styles.title}>Compose</h1>
          <p className={styles.sub}>Queue real traffic for the engine: one notification, or a batch of up to 1,000.</p>
        </div>
        <SegmentedControl
          label="What to send"
          value={tab}
          options={[
            { value: "single", label: "One notification" },
            { value: "batch", label: "Batch" },
          ]}
          onChange={(t) => setParams(t === "batch" ? { tab: "batch" } : {}, { replace: true })}
        />
      </header>
      {tab === "single" ? <SingleForm /> : <BatchForm />}
    </div>
  );
}

const priorityOptions = PRIORITIES.map((p) => ({ value: p, label: priorityLabel[p] }));
const channelOptions = CHANNELS.map((c) => {
  const Icon = channelInfo[c].Icon;
  return { value: c, label: <><Icon aria-hidden="true" /> {channelInfo[c].label}</> };
});

function SingleForm() {
  const [channel, setChannel] = useState<Channel>("sms");
  const [recipient, setRecipient] = useState("");
  const [source, setSource] = useState<"content" | "template">("content");
  const [content, setContent] = useState("");
  const [templateId, setTemplateId] = useState("");
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [priority, setPriority] = useState<Priority>("normal");
  const [scheduleAt, setScheduleAt] = useState("");
  const [key, setKey] = useState("");

  const create = useCreateNotification();
  const templates = useTemplates();
  const { mutate: renderPreview, data: preview, error: previewError, reset: resetPreview } = usePreviewTemplate();

  const info = channelInfo[channel];
  const forChannel = useMemo(() => templates.data?.data.filter((t) => t.channel === channel) ?? [], [templates.data, channel]);
  const template = forChannel.find((t) => t.id === templateId);
  const errors = create.error instanceof ApiError ? create.error.fieldErrors : {};

  useEffect(() => {
    if (source !== "template" || !template) {
      resetPreview();
      return;
    }
    const id = setTimeout(() => renderPreview({ id: template.id, variables }), 300);
    return () => clearTimeout(id);
  }, [source, template, variables, renderPreview, resetPreview]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const input: NotificationInput = { channel, recipient, priority };
    if (source === "content") input.content = content;
    else {
      input.template_id = templateId || undefined;
      input.variables = variables;
    }
    if (scheduleAt) input.scheduled_at = new Date(scheduleAt).toISOString();
    create.mutate({ input, idempotencyKey: key.trim() || undefined });
  };

  return (
    <div className={styles.layout}>
      <form className={styles.form} onSubmit={submit} noValidate>
        <Field label="Channel">
          {() => (
            <SegmentedControl label="Channel" value={channel} options={channelOptions} onChange={(c) => { setChannel(c); setTemplateId(""); }} />
          )}
        </Field>

        <Field label={info.recipientLabel} hint={info.hint} error={errors.recipient}>
          {(a) => (
            <input {...a} className={inputClass} value={recipient} onChange={(e) => setRecipient(e.target.value)} placeholder={info.placeholder} autoComplete="off" spellCheck={false} />
          )}
        </Field>

        <div className={styles.sourceSwitch}>
          <SegmentedControl
            label="Message source"
            size="sm"
            value={source}
            options={[
              { value: "content", label: "Write the message" },
              { value: "template", label: "Use a template" },
            ]}
            onChange={setSource}
          />
        </div>

        {source === "content" ? (
          <Field label="Message" error={errors.content} aside={<ContentMeter channel={channel} content={content} />}>
            {(a) => (
              <textarea {...a} className={inputClass} rows={5} value={content} onChange={(e) => setContent(e.target.value)} placeholder="What should the recipient read?" />
            )}
          </Field>
        ) : (
          <>
            <Field
              label="Template"
              error={errors.template_id}
              hint={forChannel.length === 0 ? <>No {info.label} template yet. <Link to="/templates">Create one</Link>.</> : undefined}
            >
              {(a) => (
                <select {...a} className={inputClass} value={templateId} onChange={(e) => { setTemplateId(e.target.value); setVariables({}); }}>
                  <option value="">Choose a template</option>
                  {forChannel.map((t) => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
              )}
            </Field>
            {template && template.variables.length > 0 && (
              <div className={styles.vars}>
                {template.variables.map((v) => (
                  <Field key={v} label={v}>
                    {(a) => (
                      <input {...a} className={inputClass} value={variables[v] ?? ""} onChange={(e) => setVariables({ ...variables, [v]: e.target.value })} />
                    )}
                  </Field>
                ))}
              </div>
            )}
            {template && (
              <div className={styles.preview} aria-live="polite">
                <p className={styles.previewLabel}>Rendered by the server</p>
                {preview ? (
                  <p className={styles.previewText}>{preview.content}</p>
                ) : previewError instanceof ApiError ? (
                  <p className={styles.previewMissing}>{previewError.fieldErrors.variables ?? previewError.message}</p>
                ) : (
                  <p className={styles.previewMissing}>Rendering…</p>
                )}
              </div>
            )}
            {errors.variables && <p className={styles.fieldError}>{errors.variables}</p>}
          </>
        )}

        <div className={styles.pair}>
          <Field label="Priority" error={errors.priority}>
            {() => <SegmentedControl label="Priority" value={priority} options={priorityOptions} onChange={setPriority} />}
          </Field>
          <Field label="Deliver at" hint="Leave empty to send now; up to 30 days ahead." error={errors.scheduled_at}>
            {(a) => <input {...a} type="datetime-local" className={inputClass} value={scheduleAt} onChange={(e) => setScheduleAt(e.target.value)} />}
          </Field>
        </div>

        <Field
          label="Idempotency key"
          hint="Optional. Sending again with the same key returns the original notification instead of a duplicate."
          error={errors.idempotency_key}
          aside={
            <button type="button" className={styles.linkButton} onClick={() => setKey(crypto.randomUUID())}>
              <Shuffle size={12} aria-hidden="true" /> Generate
            </button>
          }
        >
          {(a) => <input {...a} className={`${inputClass} data`} value={key} onChange={(e) => setKey(e.target.value)} placeholder="order-1042-shipped" />}
        </Field>

        {create.isError && <ProblemNotice error={create.error} hideFields={Object.keys(errors).length > 0} />}
        <div className={styles.submit}>
          <Button type="submit" variant="primary" icon={<Send />} busy={create.isPending}>
            Queue notification
          </Button>
        </div>
      </form>

      <aside className={styles.side} aria-live="polite">
        {create.data ? (
          <SentResult id={create.data.notification.id} replayed={create.data.replayed} />
        ) : (
          <div className={styles.explain}>
            <h2 className={styles.sideTitle}>What happens next</h2>
            <ol>
              <li>The API validates the message and answers <code>202 Accepted</code>.</li>
              <li>It waits in the queue by priority, or until its scheduled time.</li>
              <li>A worker delivers it within the {info.label} rate limit, retrying transient failures.</li>
              <li>Its status changes arrive here live.</li>
            </ol>
          </div>
        )}
      </aside>
    </div>
  );
}

function ContentMeter({ channel, content }: { channel: Channel; content: string }) {
  const length = [...content].length;
  const max = channelInfo[channel].maxLength;
  const over = length > max;
  if (channel !== "sms") {
    return <span data-over={over || undefined} className={styles.meter}>{formatCount(length)} / {formatCount(max)}</span>;
  }
  const sms = smsSegments(content);
  return (
    <span data-over={over || undefined} className={styles.meter} title={sms.encoding === "UCS-2" ? "Characters outside GSM-7 (for example ş, ğ, ı or emoji) limit a segment to 70 characters." : undefined}>
      {formatCount(length)} / {formatCount(max)} · {sms.segments} {sms.segments === 1 ? "segment" : "segments"} · {sms.encoding}
    </span>
  );
}

function SentResult({ id, replayed }: { id: string; replayed: boolean }) {
  const n = useNotification(id);
  return (
    <div className={styles.result}>
      <h2 className={styles.sideTitle}>{replayed ? "Already accepted earlier" : "Accepted"}</h2>
      {replayed && (
        <p className={styles.replay}>
          This idempotency key was used before, so the API returned the original notification and queued nothing new.
        </p>
      )}
      <p className="data">{shortId(id)}</p>
      {n.data && <StatusMark status={n.data.status} />}
      <Link to={`/notifications?id=${id}`}>Open its details and attempts</Link>
    </div>
  );
}

const cycle: Priority[] = ["high", "normal", "normal", "low"];

function BatchForm() {
  const [count, setCount] = useState(200);
  const [channels, setChannels] = useState<Channel[]>(["sms", "email", "push"]);
  const [mode, setMode] = useState<"mixed" | Priority>("mixed");
  const [key, setKey] = useState("");
  const create = useCreateBatch();

  const valid = count >= 1 && count <= 1000 && channels.length > 0;
  const perChannel = channels.map((c, i) => [c, Math.floor(count / channels.length) + (i < count % channels.length ? 1 : 0)] as const);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!valid) return;
    const items = synthesize(count, channels, mode === "mixed" ? (i) => cycle[i % cycle.length] ?? "normal" : () => mode);
    create.mutate({ items, idempotencyKey: key.trim() || undefined });
  };

  return (
    <div className={styles.layout}>
      <form className={styles.form} onSubmit={submit} noValidate>
        <Field label="How many" hint="1 to 1,000 per batch; the batch is accepted whole or not at all." error={count < 1 || count > 1000 ? "Choose between 1 and 1,000." : undefined}>
          {(a) => (
            <input {...a} type="number" min={1} max={1000} className={`${inputClass} ${styles.count}`} value={count} onChange={(e) => setCount(Number(e.target.value))} />
          )}
        </Field>
        <Field label="Channels" error={channels.length === 0 ? "Pick at least one channel." : undefined}>
          {() => (
            <div className={styles.chips}>
              {CHANNELS.map((c) => {
                const Icon = channelInfo[c].Icon;
                const on = channels.includes(c);
                return (
                  <Chip key={c} pressed={on} onClick={() => setChannels(on ? channels.filter((x) => x !== c) : [...channels, c])}>
                    <Icon aria-hidden="true" /> {channelInfo[c].label}
                  </Chip>
                );
              })}
            </div>
          )}
        </Field>
        <Field label="Priority">
          {() => (
            <SegmentedControl
              label="Priority"
              value={mode}
              options={[{ value: "mixed", label: "Mixed" }, ...priorityOptions]}
              onChange={setMode}
            />
          )}
        </Field>
        <Field label="Idempotency key" hint="Optional. Resend the same batch with the same key to see it replayed rather than duplicated.">
          {(a) => <input {...a} className={`${inputClass} data`} value={key} onChange={(e) => setKey(e.target.value)} placeholder="campaign-2026-11-21" />}
        </Field>
        <p className={styles.summary}>
          {valid
            ? `${formatCount(count)} synthetic notifications: ${perChannel.map(([c, n]) => `${formatCount(n)} ${channelInfo[c].label}`).join(", ")}; ${mode === "mixed" ? "mixed priorities" : `${priorityLabel[mode].toLowerCase()} priority`}.`
            : "Adjust the batch to continue."}
        </p>
        {create.isError && <ProblemNotice error={create.error} />}
        <div className={styles.submit}>
          <Button type="submit" variant="primary" icon={<Send />} busy={create.isPending} disabled={!valid}>
            Queue batch
          </Button>
        </div>
      </form>
      <aside className={styles.side} aria-live="polite">
        {create.data ? (
          <BatchResult id={create.data.id} />
        ) : (
          <div className={styles.explain}>
            <h2 className={styles.sideTitle}>Watching a batch</h2>
            <p>
              A batch shares one ID, so its progress can be followed and its waiting notifications cancelled together.
              Priorities still order delivery: high-priority items go first whatever their position in the batch.
            </p>
          </div>
        )}
      </aside>
    </div>
  );
}

function BatchResult({ id }: { id: string }) {
  const batch = useBatch(id);
  return (
    <div className={styles.result}>
      <h2 className={styles.sideTitle}>Batch queued</h2>
      <p className="data">{shortId(id)}</p>
      {batch.data && <BatchProgress batch={batch.data} />}
      <Link to={`/batches/${id}`}>Follow this batch</Link>
    </div>
  );
}
