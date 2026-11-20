---
version: 1
slug: "web-src-pages-overviewpage-tsx"
primary_target: "web/src/pages/OverviewPage.tsx"
related_targets: ["web/src/pages/NotificationsPage.tsx","web/src/pages/ComposePage.tsx"]
---

## Scope

Operator console in `web/`, Operate mode. Primary surface: the Overview at `/`. The Notifications, Compose, Batch, Templates and Provider screens inherit it.

## Audience and job

On-call and operations engineers, plus assessment reviewers. They watch delivery health through bursts and provider outages, fire test bursts, break the provider on purpose, drill from an aggregate signal to one notification's attempt history and trace, and cancel or retry safely.

## Constraints

Insider One palette (orange-red on aubergine-black, warm off-white), no logo, no official claims. Avoid an admin-kit look, flashiness and wall-of-numbers density. Must expose a burst sender, a provider chaos control and trace links. Everything shown is read from the live API.

## Direction contract

THESIS: The console is a live news gallery, a multiviewer of channel monitors where colour appears only where something is on air. It refuses the KPI-card row plus line-chart admin dashboard.

OWN-WORLD: Gallery black-aubergine ground, monitor panels one step lighter, one-pixel rules, square corners. Tally orange-red means sending, amber means probing (half-open), an alarm outline with an OPEN label means a broken circuit, mint confirms sent; everything else stays neutral. Red Hat Text for interface, Red Hat Mono with tabular figures for timecodes, IDs and counts. Segmented level meters with a ceiling mark; under-monitor label strips.

STORY: At a glance the operator sees which channels are on air, how close each is to its 100/s ceiling, what waits by priority and whether the provider is healthy. They fire a burst or break the provider and watch the gallery react, then open any as-run line to read its attempts and trace.

FIRST VIEWPORT: Top bar with name, stream tally, window, provider switch (Normal, Flaky, Outage) and the Burst action. Three equal channel monitors across the width: level meter against the ceiling, queue by priority, outcomes, latency, label strip with circuit state. Below, the throughput plot (two thirds, ceiling line) beside the timecoded as-run log (one third).

FORM: Broadcast Gallery (TV control room: multiviewer, tally lamps, level meters, as-run log), position 6 of 7 on my ranked list, seed key 722de321. Raises: only live-data marks; one multiviewer grid reflowing by whole cells; focusing a monitor dims the others and filters the as-run log; one throughput plot owns the overview; crisp printed state marks, no soft pills. Signature interaction: tallies and meters respond live to the event stream.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
