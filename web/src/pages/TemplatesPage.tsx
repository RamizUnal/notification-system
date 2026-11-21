import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import { Plus, Save, Trash2 } from "lucide-react";

import { ApiError, type Channel, type Template } from "../api/client";
import { useDeleteTemplate, usePreviewTemplate, useSaveTemplate, useTemplates } from "../api/queries";
import { Button } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { inputClass } from "../components/inputClass";
import { ProblemNotice } from "../components/ProblemNotice";
import { SegmentedControl } from "../components/SegmentedControl";
import { Skeleton } from "../components/Skeleton";
import { useToast } from "../components/Toasts";
import { CHANNELS, channelInfo } from "../lib/channels";
import { formatDateTime } from "../lib/format";
import styles from "./TemplatesPage.module.css";

const placeholder = /\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g;

export function TemplatesPage() {
  const templates = useTemplates();
  const [params, setParams] = useSearchParams();
  const selectedId = params.get("t");
  const list = templates.data?.data ?? [];
  const selected = list.find((t) => t.id === selectedId);
  const creating = selectedId === "new" || (templates.isSuccess && list.length === 0);

  return (
    <div className={styles.page}>
      <header className={styles.head}>
        <div>
          <h1 className={styles.title}>Templates</h1>
          <p className={styles.sub}>Reusable messages with {"{{variable}}"} placeholders, rendered when a notification is created.</p>
        </div>
        <Button icon={<Plus />} onClick={() => setParams({ t: "new" })}>
          New template
        </Button>
      </header>

      {templates.isError && <ProblemNotice error={templates.error} onRetry={() => void templates.refetch()} />}

      <div className={styles.layout}>
        <nav className={styles.list} aria-label="Templates">
          {templates.isPending && [1, 2, 3].map((i) => <Skeleton key={i} height={52} />)}
          {templates.isSuccess && list.length === 0 && (
            <p className={styles.none}>No templates yet. Write the first one on the right.</p>
          )}
          {list.map((t) => (
            <button
              key={t.id}
              type="button"
              className={styles.item}
              aria-current={t.id === selectedId || undefined}
              onClick={() => setParams({ t: t.id })}
            >
              <span className={`${styles.itemName} data`}>{t.name}</span>
              <span className={styles.itemMeta}>
                {channelInfo[t.channel].label} · {t.variables.length} {t.variables.length === 1 ? "variable" : "variables"}
              </span>
            </button>
          ))}
        </nav>

        <div>
          {creating ? (
            <TemplateEditor key="new" onSaved={(t) => setParams({ t: t.id })} />
          ) : selected ? (
            <TemplateEditor key={selected.id} template={selected} onSaved={(t) => setParams({ t: t.id })} onDeleted={() => setParams({})} />
          ) : (
            templates.isSuccess && (
              <EmptyState title="Pick a template to edit it">
                Or create a new one. Notifications can then reference a template and pass the values for its placeholders.
              </EmptyState>
            )
          )}
        </div>
      </div>
    </div>
  );
}

interface EditorProps {
  template?: Template;
  onSaved: (t: Template) => void;
  onDeleted?: () => void;
}

function TemplateEditor({ template, onSaved, onDeleted }: EditorProps) {
  const [name, setName] = useState(template?.name ?? "");
  const [channel, setChannel] = useState<Channel>(template?.channel ?? "sms");
  const [body, setBody] = useState(template?.body ?? "");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const save = useSaveTemplate();
  const remove = useDeleteTemplate();
  const toast = useToast();

  const variables = useMemo(() => [...new Set([...body.matchAll(placeholder)].map((m) => m[1] ?? ""))], [body]);
  const errors: Record<string, string> = save.error instanceof ApiError ? { ...save.error.fieldErrors } : {};
  if (save.error instanceof ApiError && save.error.problem.code === "template_name_taken") {
    errors.name = "Another template already uses this name.";
  }

  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(
      { id: template?.id, input: { name, channel, body } },
      {
        onSuccess: (t) => {
          toast({ tone: "clear", title: template ? "Template saved" : "Template created" });
          onSaved(t);
        },
      },
    );
  };

  return (
    <div className={styles.editor} data-single={!template || undefined}>
      <form className={styles.form} onSubmit={submit} noValidate>
        <h2 className={styles.editorTitle}>{template ? "Edit template" : "New template"}</h2>
        <Field label="Name" hint="Lowercase letters, digits, - and _, for example order-shipped." error={errors.name}>
          {(a) => <input {...a} className={`${inputClass} data`} value={name} onChange={(e) => setName(e.target.value)} placeholder="order-shipped" />}
        </Field>
        <Field label="Channel" error={errors.channel}>
          {() => (
            <SegmentedControl
              label="Channel"
              value={channel}
              options={CHANNELS.map((c) => ({ value: c, label: channelInfo[c].label }))}
              onChange={setChannel}
            />
          )}
        </Field>
        <Field
          label="Body"
          error={errors.body}
          hint="Write {{name}} wherever a value goes."
          aside={`${[...body].length} / ${channelInfo[channel].maxLength}`}
        >
          {(a) => (
            <textarea
              {...a}
              className={inputClass}
              rows={6}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder="Hi {{name}}, your order {{order_id}} is on its way."
            />
          )}
        </Field>
        {variables.length > 0 && (
          <p className={styles.vars}>
            Placeholders:{" "}
            {variables.map((v) => (
              <code key={v} className={styles.var}>
                {v}
              </code>
            ))}
          </p>
        )}
        {save.isError && Object.keys(errors).length === 0 && <ProblemNotice error={save.error} />}
        {remove.isError && <ProblemNotice error={remove.error} />}
        <div className={styles.actions}>
          {template && onDeleted && (
            confirmDelete ? (
              <span className={styles.confirm}>
                Delete {template.name}? Sent notifications keep their text.
                <Button size="sm" variant="danger" busy={remove.isPending} onClick={() => remove.mutate(template.id, { onSuccess: onDeleted })}>
                  Delete
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setConfirmDelete(false)}>
                  Keep
                </Button>
              </span>
            ) : (
              <Button size="sm" variant="ghost" icon={<Trash2 />} onClick={() => setConfirmDelete(true)}>
                Delete
              </Button>
            )
          )}
          <Button type="submit" icon={<Save />} busy={save.isPending}>
            {template ? "Save changes" : "Create template"}
          </Button>
        </div>
        {template && <p className={styles.meta}>Last changed {formatDateTime(template.updated_at)}</p>}
      </form>
      {template && <Preview template={template} />}
    </div>
  );
}

function Preview({ template }: { template: Template }) {
  const [values, setValues] = useState<Record<string, string>>({});
  const { mutate, data, error } = usePreviewTemplate();
  useEffect(() => {
    const id = setTimeout(() => mutate({ id: template.id, variables: values }), 300);
    return () => clearTimeout(id);
  }, [template.id, values, mutate]);

  return (
    <section className={styles.previewPanel} aria-labelledby="preview-title">
      <h2 id="preview-title" className={styles.editorTitle}>
        Preview
      </h2>
      {template.variables.length > 0 ? (
        <div className={styles.previewVars}>
          {template.variables.map((v) => (
            <Field key={v} label={v}>
              {(a) => <input {...a} className={inputClass} value={values[v] ?? ""} onChange={(e) => setValues({ ...values, [v]: e.target.value })} />}
            </Field>
          ))}
        </div>
      ) : (
        <p className={styles.meta}>This template has no placeholders.</p>
      )}
      <div className={styles.rendered} aria-live="polite">
        {data ? (
          <>
            <p className={styles.renderedText}>{data.content}</p>
            <p className={styles.meta}>{data.length} characters, rendered by the server</p>
          </>
        ) : error instanceof ApiError ? (
          <p className={styles.meta}>{error.fieldErrors.variables ?? error.message}</p>
        ) : (
          <p className={styles.meta}>Rendering…</p>
        )}
      </div>
    </section>
  );
}
