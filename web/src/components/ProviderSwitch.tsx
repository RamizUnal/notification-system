import { modeOf, modeSettings, useProviderStats, useUpdateProvider, type ProviderMode } from "../api/provider";
import { formatPercent } from "../lib/format";
import { SegmentedControl } from "./SegmentedControl";
import { useToast } from "./Toasts";
import styles from "./ProviderSwitch.module.css";

const modes = [
  { value: "stable", label: "Stable" },
  { value: "flaky", label: "Flaky" },
  { value: "outage", label: "Outage" },
] as const;

const announce: Record<ProviderMode, string> = {
  stable: "Provider stable: occasional transient failures",
  flaky: "Provider flaky: about a third of sends fail and are retried",
  outage: "Provider down: watch the circuits open",
};

/** Breaks the simulated provider on purpose, to show retries and circuit breaking. */
export function ProviderSwitch() {
  const stats = useProviderStats(2000);
  const update = useUpdateProvider();
  const toast = useToast();

  if (stats.isError) {
    return (
      <span className={styles.external} title="The bundled provider simulator is not reachable; deliveries go to an external provider such as webhook.site.">
        External provider
      </span>
    );
  }
  const settings = stats.data?.settings;
  const mode = settings ? modeOf(settings) : "stable";

  return (
    <div className={styles.switch}>
      <SegmentedControl
        label="Provider behaviour"
        size="sm"
        value={mode}
        options={modes.map((m) => ({ ...m, disabled: !settings || update.isPending }))}
        onChange={(next) => {
          if (!settings || next === mode) return;
          update.mutate(
            { ...settings, ...modeSettings[next] },
            { onSuccess: () => toast({ tone: next === "outage" ? "alarm" : next === "flaky" ? "probe" : "clear", title: announce[next] }) },
          );
        }}
      />
      {settings && !settings.outage && (
        <span className={styles.rate} title="Share of sends the simulator fails with 503">
          {formatPercent(settings.failure_rate, 0)} fail
        </span>
      )}
    </div>
  );
}
