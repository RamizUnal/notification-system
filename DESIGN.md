---
name: Dispatch
description: A broadcast-gallery operator console for a live notification delivery pipeline.
colors:
  ground: "#141016"
  console-black: "#100c11"
  mullion: "#0b080c"
  panel: "#1c171f"
  panel-2: "#241e28"
  panel-3: "#2e2733"
  rule: "#352d3a"
  rule-strong: "#4b4151"
  ink: "#efebe6"
  ink-2: "#c4bbc6"
  ink-3: "#998f9e"
  ink-inverse: "#141016"
  on-air: "#f04e23"
  on-air-bright: "#ff6a43"
  on-air-soft: "rgba(240, 78, 35, 0.14)"
  probe: "#f2b33d"
  probe-soft: "rgba(242, 179, 61, 0.14)"
  clear: "#5fe0be"
typography:
  headline:
    fontFamily: "Red Hat Text Variable, Red Hat Text, system-ui, sans-serif"
    fontSize: "1.625rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "-0.01em"
  title:
    fontFamily: "Red Hat Text Variable, Red Hat Text, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.25
  body:
    fontFamily: "Red Hat Text Variable, Red Hat Text, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Red Hat Text Variable, Red Hat Text, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 600
    lineHeight: 1.5
  caption:
    fontFamily: "Red Hat Text Variable, Red Hat Text, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.5
  readout:
    fontFamily: "Red Hat Mono Variable, Red Hat Mono, ui-monospace, monospace"
    fontSize: "1.3125rem"
    fontWeight: 600
    lineHeight: 1
    fontFeature: "tnum"
  data:
    fontFamily: "Red Hat Mono Variable, Red Hat Mono, ui-monospace, monospace"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.5
    fontFeature: "tnum"
  strip:
    fontFamily: "Red Hat Mono Variable, Red Hat Mono, ui-monospace, monospace"
    fontSize: "0.75rem"
    fontWeight: 700
    lineHeight: 1
    letterSpacing: "0.06em"
  scale-mark:
    fontFamily: "Red Hat Mono Variable, Red Hat Mono, ui-monospace, monospace"
    fontSize: "0.6875rem"
    fontWeight: 400
    lineHeight: 1
rounded:
  none: "0px"
spacing:
  s-1: "4px"
  s-2: "8px"
  s-3: "12px"
  s-4: "16px"
  s-5: "24px"
  s-6: "32px"
components:
  button-primary:
    backgroundColor: "{colors.on-air}"
    textColor: "{colors.ink-inverse}"
    typography: "{typography.label}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  button-primary-hover:
    backgroundColor: "{colors.on-air-bright}"
  button-secondary:
    backgroundColor: "{colors.panel-2}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  button-secondary-hover:
    backgroundColor: "{colors.panel-3}"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.ink-2}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  button-ghost-hover:
    backgroundColor: "{colors.panel-2}"
    textColor: "{colors.ink}"
  button-danger:
    backgroundColor: "transparent"
    textColor: "{colors.on-air}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  button-danger-hover:
    backgroundColor: "{colors.on-air-soft}"
  button-sm:
    padding: "0 12px"
    height: "28px"
  input:
    backgroundColor: "{colors.panel-2}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.none}"
    padding: "7px 12px"
    height: "36px"
  segment:
    backgroundColor: "transparent"
    textColor: "{colors.ink-2}"
    rounded: "{rounded.none}"
    padding: "0 12px"
    height: "32px"
  segment-selected:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.ink-inverse}"
  segment-sm:
    padding: "0 10px"
    height: "26px"
  chip:
    backgroundColor: "transparent"
    textColor: "{colors.ink-2}"
    rounded: "{rounded.none}"
    padding: "0 10px"
    height: "28px"
  chip-pressed:
    backgroundColor: "{colors.panel-3}"
    textColor: "{colors.ink}"
  channel-monitor:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    padding: "24px 24px 16px"
  monitor-strip:
    backgroundColor: "{colors.console-black}"
    textColor: "{colors.ink}"
    typography: "{typography.strip}"
    padding: "0 16px"
    height: "40px"
  monitor-strip-focused:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.ink-inverse}"
  level-meter:
    backgroundColor: "{colors.panel-3}"
    width: "22px"
  as-run-row:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.data}"
    padding: "0 16px"
    height: "30px"
  as-run-row-hover:
    backgroundColor: "{colors.panel-2}"
  stream-tally:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.strip}"
    padding: "0 12px"
    height: "28px"
  top-bar:
    backgroundColor: "{colors.console-black}"
    textColor: "{colors.ink-2}"
    padding: "0 24px"
    height: "56px"
  drawer:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    padding: "24px"
    width: "min(480px, 100vw)"
  toast:
    backgroundColor: "{colors.panel-2}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    padding: "12px 12px 12px 16px"
    width: "min(380px, calc(100vw - 32px))"
  problem-notice:
    backgroundColor: "{colors.on-air-soft}"
    textColor: "{colors.ink}"
    typography: "{typography.label}"
    rounded: "{rounded.none}"
    padding: "12px 16px"
---

# Design System: Dispatch

## Overview

**Creative North Star: "The Broadcast Gallery"**

The console is a news gallery's control room: a dim aubergine-black wall, a multiviewer of channel monitors butted edge to edge, a label strip under each screen, segmented level meters read against a ceiling, and an as-run log ticking down the side. The room is neutral on purpose. Colour appears only where something is on air, probing, broken or confirmed, so the operator's eye lands on trouble before it reads a single word.

Density is that of an instrument panel, not a report: every number is a live reading set in a measurement face, every surface sits one tonal step above the one behind it, and every division is a one-pixel rule. Nothing is rounded, nothing glows, nothing floats except the few layers that genuinely sit over the wall (the drawer, toasts, the burst menu). State is printed: square lamps, outlines, hatching and short labels, never soft pills.

The world refuses the KPI-card row with a line chart under it. Aggregates live inside the monitors they describe, and the one throughput plot on the overview is a strip-chart with a live "now" edge, not a dashboard tile.

**Key Characteristics:**
- Gallery-black ground with monitor panels one tonal step lighter; hairline rules divide everything.
- Three tally hues (on-air, probe, clear), each a state; everything else is warm off-white ink at three strengths.
- Red Hat Text for words, Red Hat Mono with tabular figures for every timecode, ID, count, rate and latency.
- Square corners throughout; zero radius.
- Broadcast instruments as components: monitor wall, under-monitor strip, level meter with ceiling and ticks, as-run log, tally lamps.
- Motion is short and state-driven: tallies, meters and log rows respond to the event stream.

## Colors

A dim, warm-neutral room in which three saturated tally hues are the only colour, and each hue is a state.

### Primary
- **Tally Orange-Red** (on-air): the on-air tally. A channel monitor's frame and lamp while it is sending, the "Processing" lamp, the hot top segments of a level meter at the ceiling, failure ticks on the throughput plot, failed-attempt counts, the circuit-open outline and hatch, problem notices, the Live stream lamp, and the fill of the action that puts traffic on air (Send burst, Send).
- **Hot Tally** (on-air-bright): hover state of the on-air primary action only.
- **Tally Wash** (on-air-soft): a 14% wash of the on-air hue behind alarm surfaces: the circuit-open hatch, problem notices, the drawer's last-error band, the danger button's hover.

### Secondary
- **Probe Amber** (probe): half-open circuit (monitor frame and lamp "Probing"), the Retrying status, the stream "Reconnecting" lamp, and retry attempt numbers in the as-run log.
- **Probe Wash** (probe-soft): a 14% amber wash for a cautionary aside in the compose form.

### Tertiary
- **Confirm Mint** (clear): confirmed sent. Used only as the lamp beside "Sent" and the confirmation toast when the provider returns to stable. It never fills a surface.

### Neutral
- **Gallery Black** (ground): the page wall, the scrollbar track, recessed message bodies in the drawer.
- **Console Black** (console-black): the top bar and the under-monitor strips, one step darker than the wall so the strips read as one band.
- **Mullion** (mullion): the 2px seam between monitors in the multiviewer wall.
- **Monitor** (panel): monitor tiles, the plot and log panels, the drawer.
- **Raised** (panel-2): inputs, secondary buttons, toasts, menus, readouts and hover rows.
- **Well** (panel-3): unlit meter segments, queue tracks, pressed chips, the as-run arrival flash.
- **Rule** (rule): every hairline divider and panel border.
- **Strong Rule** (rule-strong): control borders (inputs, segmented controls, chips, tally box), the drawer edge, the now-edge on the plot, scrollbar thumb.
- **Ink** (ink): primary text, the selected segment fill, the focused monitor strip fill, the focus outline, the peak-hold meter segment, the SMS plot line.
- **Ink 2** (ink-2): secondary text, lit meter segments, queue fills, email and push plot lines, the ceiling line.
- **Ink 3** (ink-3): timecodes, axis labels, placeholders, meter floor marks, unlit lamp outlines, the dashed no-worker frame.
- **Ink Inverse** (ink-inverse): text on ink or on-air fills.

### Named Rules
**The Tally Rule.** Colour marks state and nothing else: on-air for sending and for failures that need eyes, probe for half-open and retrying, clear for confirmed sent. Queued, idle, cued, scheduled and cancelled stay neutral. If a coloured mark cannot be named as one of those states, it is grey.

**The Alarm-By-Form Rule.** A broken circuit or permanent failure reuses the on-air hue; it is told apart from "on air" by form, not by a fourth colour: diagonal hatching on the monitor, a slashed lamp, bold text and an explicit label ("Circuit open", "Failed").

**The Dim Room Rule.** The ground and panels stay at the darkest steps; a quiet console is nearly monochrome, and a screen with no traffic shows no colour at all apart from the primary action.

## Typography

**Interface Font:** Red Hat Text Variable (with Red Hat Text, system-ui, sans-serif)
**Data Font:** Red Hat Mono Variable (with Red Hat Mono, ui-monospace, monospace)

**Character:** One humanist family in two voices: the Text cut speaks in words, the Mono cut speaks in measurements. Both are self-hosted variable fonts; weights in use are 400, 500, 600 and 700.

### Hierarchy
- **Headline** (600, 1.625rem, 1.25, -0.01em): page titles only (Overview, Notifications).
- **Title** (600, 1.125rem, 1.25): the drawer title.
- **Readout** (Mono 600, 1.3125rem, 1): the big live figures in a monitor (rate, waiting count).
- **Body** (400, 0.875rem, 1.5): the base interface size; section and panel headings use it at 600.
- **Label** (600, 0.8125rem): field labels, monitor terms, buttons in small size, notices.
- **Caption** (400, 0.75rem): secondary lines, dt terms, hints, legends, priority rows.
- **Data** (Mono, tabular figures): timecodes, IDs, counts, latencies, HTTP codes, the house clock (600, 0.875rem, 0.04em).
- **Strip** (Mono 600-700, 0.75-0.8125rem, 0.06em, uppercase): under-monitor strip and the stream tally only.
- **Scale Mark** (Mono, 0.6875rem / 11px): meter ceiling and floor numerals, plot axes, ceiling and now labels.

The scale steps are 11, 12, 13, 14, 16, 18, 21 and 26px; the interface lives in the 12-14px band.

### Named Rules
**The Measurement Rule.** Every timecode, identifier, count, rate and latency is set in Red Hat Mono with tabular figures, so columns hold still as values tick.

**The Strip Caps Rule.** Uppercase with 0.06em tracking belongs to the under-monitor strip and the stream tally, the broadcast label devices. Headings, labels and buttons stay in sentence case.

## Layout

A single centred column (max 1680px, 24px padding, 16px on phones) under a sticky 56px top bar. Spacing follows a 4px rhythm (4, 8, 12, 16, 24, 32px); page sections stack at 24px, panels sit 12px apart, and internal monitor padding is 24px.

The overview is a multiviewer: three equal monitors in one grid with a 2px mullion gap and 2px frame, so the under-monitor strips line up as a single band. Below it, the throughput plot takes two thirds and the as-run log one third (minimum 320px).

Responsive behaviour reflows by whole cells, never by squeezing:
- At 1280px the brand subtitle drops.
- At 1200px the plot and log stack.
- At 1100px the top bar wraps; navigation becomes a full-width horizontally scrolling tab row with a faded trailing edge, the house clock hides, and the drawer becomes full height over the bar.
- At 1024px the monitor wall becomes one column.
- At 960px and 760px two-column forms (Compose, Provider, Templates) stack.
- At 900px the notifications table drops the content column; at 640px each row becomes a two-line record, filters fold behind a toggle, the top bar unsticks, the stream tally collapses to its lamp and word, the provider switch hides from the bar, and the burst menu becomes a fixed sheet below the bar.
- At 560px monitor padding tightens to 16px and the strip drops its detail text.

### Named Rules
**The Whole-Cell Reflow Rule.** Grids lose columns, never shrink monitors below legibility; a monitor is always a whole monitor.

## Elevation & Depth

The wall is flat. Depth comes from tonal steps (ground, panel, panel-2, panel-3) and hairline rules, not shadows. Only layers that float over the wall carry a soft, dark ambient shadow.

### Shadow Vocabulary
- **Drawer edge** (`box-shadow: -24px 0 48px rgba(0, 0, 0, 0.35)`): the notification drawer, cast leftward over the list.
- **Toast** (`box-shadow: 0 12px 32px rgba(0, 0, 0, 0.45)`): toasts in the bottom-right corner.
- **Menu** (`box-shadow: 0 16px 40px rgba(0, 0, 0, 0.5)`): the burst menu.

### Named Rules
**The Flat Wall Rule.** Anything mounted on the wall (monitors, panels, tables, inputs) has no shadow. A shadow means the layer floats and can be dismissed.

## Shapes

Square everything: zero radius on panels, buttons, inputs, chips, lamps, toasts and the drawer. Borders are one-pixel hairlines; tallies are 2px inset outlines on the monitor frame. Lamps are 8-9px squares: filled for an active state, outlined for muted, slashed for alarm. Hatching (-45deg, 6px on, 8px off, in on-air wash) marks a broken circuit. The brand mark is a square outline with a square inset.

### Named Rules
**The Square Corner Rule.** Radius is 0 everywhere. A rounded corner or a pill shape is foreign to this room.

## Components

### Multiviewer Monitor Wall
Three channel monitors on a mullion-black seam. Each monitor is a panel-coloured tile with a 2px inset outline that is the tally: transparent when idle or cued, on-air when sending, probe when half-open, on-air plus hatch when the circuit is open, dashed ink-3 when no worker reports. Focusing a monitor (clicking its strip) dims the others to 42% opacity and filters the as-run log and plot to that channel. Inside: the level meter column on the left; waiting count with priority tracks (6px well, ink-2 fill scaled by share), sent and failed counts (failed in on-air), success rate and p50/p95/p99 latency on the right, separated by a hairline.

### Under-Monitor Strip
A 40px console-black band under each monitor: channel icon and name on the left, lamp plus state label plus detail on the right, all in Mono uppercase with 0.06em tracking (detail in sentence case, ink-2). It is the monitor's focus button; focused, it inverts to an ink fill with inverse text.

### Level Meter
A 20-segment vertical column, 22px wide, with the ceiling numeral (100) above a hairline cap and 0 below. Unlit segments are wells; lit segments are ink-2; segments at the ceiling go on-air; a peak-hold segment is ink. Tick marks at 25%, 50% (stronger) and 75% sit 3px to the right. Segment colour changes in 90ms linear.

### Tally Lamps and Status Marks
A square lamp (8px) plus a word. Tones: neutral (ink-3 fill), muted (outline only, ink-2 text), live (on-air), probe (amber), clear (mint), alarm (slashed outlined lamp, on-air bold text). The stream tally in the top bar is a 28px hairline box with lamp and "LIVE / CONNECTING / RECONNECTING" in Mono caps.

### As-Run Log
A panel with a hairline head (title, scope, Pause) and a scrolling list capped at 318px. Rows are 30px, hairline-separated, on a grid of timecode (96px, ink-3), channel (48px, 600, 0.04em), status mark, and ID (right-aligned, ink-2); retry attempt numbers are amber. The newest row arrives with a well-coloured flash that fades in 220ms. Rows open the notification drawer.

### Throughput Plot
One strip-chart per overview, 1-second buckets. Hairline grid on crisp edges, Mono 11px axes. Series are distinguished by line style, not hue: SMS solid ink, email dashed ink-2 (7/4), push dotted ink-2 (1.5/4), 1.75px strokes. A dashed ceiling line labelled "ceiling 100/s per channel"; a strong-rule vertical "now" edge marks where settled data hands over to the live meter reading. Failures are on-air ticks in a strip under the axis. Hover shows a cursor line and a raised readout box (178px). A channel focus dims the other series to 18%.

### Drawer
Fixed to the right below the top bar, min(480px, 100vw) wide, panel background, strong-rule left edge, drawer shadow, slides in 24px over 220ms without fading. Head 52px with title and close; body padded 24px with 32px between sections. Sections use hairline-underlined titles, two-column fact grids, a recessed ground-coloured message block, an on-air-wash last-error band, and an attempt timeline of hairline rows.

### Buttons
- **Shape:** square (0px), 36px tall (28px small), 600 weight, 0.01em tracking, 16px icons.
- **Primary:** on-air fill with inverse text; reserved for the action that puts traffic on air. Hover brightens to on-air-bright.
- **Secondary:** raised fill with strong-rule border; hover goes to well with ink-3 border.
- **Ghost:** transparent ink-2; hover raised with ink text.
- **Danger:** transparent with on-air border and text; hover takes the on-air wash. Destructive actions (cancel, delete) confirm in place before acting.
- **Disabled / busy:** 45% / 75% opacity.

### Segmented Controls
A hairline strong-rule box of butted segments divided by hairlines (32px, or 26px small). The checked segment inverts to an ink fill with inverse text at 600; hover on others is raised with ink text. Used for the time window and the provider switch (Stable, Flaky, Outage).

### Chips
28px filter toggles with a strong-rule hairline and ink-2 text; pressed chips take a well fill with an ink border. Status chips carry their lamp.

### Inputs / Fields
- **Style:** raised fill, strong-rule hairline, 0 radius, 36px minimum; label 600 above, hint and counter in caption ink-2.
- **Focus:** ink border plus a 1px ink ring. Global focus elsewhere is a 2px ink outline offset 2px.
- **Error:** on-air border and on-air caption text. Selects use a drawn chevron in ink-2.

### Problem Notices
An inline block with an on-air hairline border and on-air wash, a 600 title, optional field list and request ID in caption, and a small Retry button on the right.

### Toasts
Bottom-right stack, up to 380px wide, raised fill, strong-rule hairline, toast shadow, lamp plus title plus body, enter by rising 8px and fading in over 220ms. The lamp tone follows the Tally Rule (clear for stable, probe for flaky, alarm for outage).

### Navigation
Top bar on console black, 56px, hairline bottom rule: brand mark and name, tab links (ink-2, 500; current page ink with a 2px ink underline inset), then the house clock, stream tally, provider switch and Burst action at the right.

### Motion
One easing (`cubic-bezier(0.16, 1, 0.3, 1)`) at two durations: 150ms for control feedback (hover, press, tabs) and 220ms for state changes (tally outlines, dimming, queue tracks, arrivals, drawer, toasts). Meter segments step in 90ms linear. The only loop is the skeleton sweep (1.4s linear). Under `prefers-reduced-motion: reduce`, all animations and transitions collapse to 0.01ms and run once; state still changes, it just lands.

## Do's and Don'ts

### Do:
- **Do** keep colour for state: on-air (#f04e23) for sending and failures, probe (#f2b33d) for half-open and retrying, clear (#5fe0be) for sent.
- **Do** separate surfaces with one tonal step and a 1px rule, not a shadow.
- **Do** set every number, timecode and ID in Red Hat Mono with tabular figures.
- **Do** pair every coloured lamp with a word, so state never depends on hue alone.
- **Do** distinguish plot series by line style (solid, dashed, dotted) in neutral ink.
- **Do** reflow grids by whole cells: three monitors, then one.
- **Do** keep transitions on the single ease-out curve at 150ms or 220ms and honour reduced motion.

### Don't:
- **Don't** round corners or use pill badges; lamps and outlines are square.
- **Don't** colour idle, queued, cued, scheduled or cancelled states.
- **Don't** introduce a fourth tally hue for alarms; alarms reuse on-air with hatching, a slashed lamp and a label.
- **Don't** shadow anything mounted on the wall; only the drawer, toasts and menus float.
- **Don't** build KPI-card rows; aggregates live inside the monitor they describe.
- **Don't** use uppercase tracked text outside the under-monitor strip and the stream tally.
