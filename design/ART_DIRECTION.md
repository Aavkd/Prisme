# Prisme — design study 01

Version 0.2 · 6 September 2026 · **Direction, responsive home and Terminal / Matrix theme accepted**

This is the accepted art direction, design-system foundation, and home-screen study for the infrastructure platform. It is based on the existing product specification, especially sections 7.24–7.25 and 8.1–8.24. The HTML files remain design studies. Reusable Flutter implementation now lives in [packages/prisme_ui](../packages/prisme_ui/README.md), with a development catalogue and no production integrations.

## Direction: calm precision

A personal control room with graphite surfaces, clear typography, and restrained blue accents. Machines and workers form the main working surface. Color draws attention to evidence, activity, and requests; quiet space and alignment make dense data easier to scan.

Three principles:

1. **Readable density.** Group related facts, align numbers, and reveal secondary detail on demand.
2. **Evidence before reassurance.** Health needs a recent positive observation. An old value remains explicitly old.
3. **Attention with proportion.** A worker asking for a decision is amber. A production incident can take over the top of the screen. Routine activity stays quiet.

The working name **Prisme** follows the project directory. The lowercase wordmark and triangle are provisional, not an approved Mantara identity. French is the language of this first study; English remains a required production localization.

## Visual foundations

The accepted values are in [tokens.json](tokens.json). The visual specimens are in [foundations.html](foundations.html).

| Foundation | Proposal | Purpose |
|---|---|---|
| Background | Graphite in dark mode; cool grey in light mode | Separate the working surface from content |
| Panels | Slightly raised opaque neutral surfaces | Group information without heavy shadows |
| Accent | Cobalt; teal available as an exploratory alternative | Navigation, active work, selected controls |
| Health | Green + check + explicit state | Verified recent health |
| Attention | Amber + message or warning icon + label | A decision, partial data, or an approaching threshold |
| Incident | Red + alert icon + severity + impact | Confirmed failure or urgent impact |
| Stale / unknown | Neutral + clock + observation time | No unsupported claim about present health |
| Wordmark | Space Grotesk, 600 | Compact geometric identity |
| Interface | Inter, regular to semibold | Legible controls and headings |
| Numbers / identifiers | IBM Plex Mono, tabular figures | Stable comparison of values and units |
| Spacing | 4, 8, 12, 16, 24, 32 px | Shared rhythm; small optical adjustments allowed |
| Corners | 4 px badges; 6 px buttons; 10 px cards; 12 px dialogs | Recognizable hierarchy without exaggerated rounding |
| Elevation | Borders for cards; shadow only for overlays | Keep the dashboard visually quiet |
| Icons | Lucide, 16–20 px, outline | Consistent visual weight; triangle is only a provisional mark |
| Motion | 120 ms interaction transitions | Feedback without looping decoration; respect reduced motion |

### Type and density

Home title 28 px; section headings 15 px; primary task and machine labels 14 px; controls 13 px; supporting metadata 11–12 px; prominent metrics 27 px. The 10 px eyebrow, time-axis endpoints, and frame annotation are secondary. These are prototype sizes, to be checked on actual Windows scaling and the S24 before locking Flutter tokens.

Comfortable density uses 18 px card padding; compact uses 13 px. Both retain the same hierarchy. Mobile controls have at least a 44 px hit area in this study; the Flutter implementation should use a 48 dp minimum for Android actions and respect system text scaling. Dense metrics must not turn into smaller touch targets.

## Home screen

Editable source: [home.html](home.html). The conversation presents an interactive copy with design controls.

### Windows

1. Application shell: workspace, current environment, global search, device and connection status.
2. Heading: overview, data freshness, personalize action.
3. Priority incident or loss-of-connection banner, when applicable.
4. Compact health strip covering **all supervised resources**, including those not pinned.
5. Two primary columns: **my machines** and **my workers**.
6. Two quieter sections: project health and correlated delivery activity.
7. Backup, edge, and automation status.

Machine cards show role, health, CPU with a shared 15-minute trend, RAM, storage, receive throughput, workload count, and freshness. The whole card opens its resource preview.

Worker rows show tool, task, project, machine, observed execution state, duration, and freshness. A Claude Code request is deliberately distinct from an error. Numeric progress is shown only for the example audio pipeline, which has a known item count. The Codex example explicitly has no measured percentage. The detail preview separates local process resources from remote model compute and leaves cost, quotas, and GPU attribution unavailable.

### Android

The same hierarchy becomes a vertical layout with bottom navigation. The default device selection differs from Windows: one machine and two workers. This demonstrates that changing the phone does not overwrite the desktop selection. Card details appear in an overlay; production navigation beyond home is a visual placeholder.

The browser study is responsive, including narrow widths. It does not simulate the complete native Android system bars, biometric flow, keyboard, safe areas, or system Back behavior.

### Design controls

| Control | Variants |
|---|---|
| Device | Windows / Android (S24) |
| Appearance | Match app / dark / light / Terminal–Matrix |
| Home state | Normal / priority incident / unavailable control centre / partial telemetry / no pinned resources |
| Density | Comfortable / compact |
| Accent study | Cobalt / teal |

The foundation board has its own appearance control. Design controls belong to the review surface, not the production UI. Personalization, resource details, search, and modal dismissal are local prototype interactions. No requests leave the mockup, apart from font loading.

### Scenarios and invariants

- **Normal:** 13 healthy production services, 3 online machines, 2 executing workers plus 1 build, and 1 worker waiting for a decision. The visible machine count is the device's selection, not the global count.
- **Priority incident:** Sounding Lines' audio worker has stopped making progress. The worker, project, global summary, and incident banner agree. A proposed restart is described for review; it cannot be executed.
- **Control centre unavailable:** the last observation is 14:18; cached cards and project values are labelled old. The interface does not infer that the VPS is down. Emergency access opens a proposed independently authenticated flow with current VPS state still unknown.
- **Partial telemetry:** Atelier's CPU and RAM are available; GPU collection is absent. A missing reading is never a zero.
- **No pinned resources:** empty selection cards guide personalization. Global supervision and critical alerts remain visible.

All values, tasks, releases, times, and topology details in the mockup are **illustrative fixtures**, not a new infrastructure audit. The future tower's specifications are invented solely for exploring the card and picker. They do not replace the outstanding hardware diagnostic.

## Component contracts for this study

| Component | Variants and states | Interaction / accessibility | Required data |
|---|---|---|---|
| Status badge | Healthy, active, attention, incident, stale | Text plus icon/shape; never color alone; noninteractive | State, evidence, observed time |
| Environment badge | Production shown; future staging/dev variants | Persistent context; not an actionable dropdown when only one context is mocked | Environment identifier and label |
| Freshness label | Recent, old, partial | Readable inline; old values marked per block | Observation timestamp, source, collection status |
| Machine card | Healthy, partial, cached | Semantic button; keyboard focus; opens preview | Identity, role, metrics, capacities, time series, source timestamps |
| Worker row | Running, waiting for user, blocked, cached | Named task button; request detail; item-count progress only when known | Stable worker/task ID, state, task, machine/project, duration, evidence |
| Action button | Primary, secondary, disabled; hover/focus/pressed | Native button; visible focus; unavailable actions inert | Label, purpose, enabled reason |
| Health summary | Nominal, incident, unknown, partial | Entire supervised scope, independent of pinned cards | Aggregated current health with freshness |
| Priority banner | Incident, unavailable centre, partial collection | Impact first; context and next step; cannot be hidden by unpinning | Severity, resource, duration, impact, evidence, next step |
| Resource preview | Machine, worker, project, incident | Focus trap, Escape, close button, backdrop; focus restored | Selected resource and observation context |
| Personalization | Windows and Android selections | Labelled checkboxes; Apply; cancel with no change | Device ID, available resources, current pinned selection |
| Search | Matching resources, no results | Native search field; labelled results; Ctrl+K when mockup has focus | Fixture resource names and types |
| Empty selection | Machines or workers | Explicit selection action; global health still visible | Selection count and available inventory |

The sidebar adds **Workers** and uses **Machines** as an umbrella for the VPS, tower, and personal computers. This is a proposed information-architecture refinement, not an approved replacement for the specification's navigation.

## Scope of the HTML studies

This pass covers the visual foundations, home in two device layouts, five home scenarios, and selected component interactions. Loading skeletons, authorization failures, action execution states, real-time transitions, translations, configurable metric sets/reordering, and complete resource pages remain for subsequent screen design. They are not claimed to be implemented.

The user accepted the direction, layout and Matrix addition on 6 September 2026. Cobalt remains the classic baseline; teal is an archived exploration. The following are retained as the original review topics:

- The overall personality and cobalt versus teal accent.
- Home density and the balance between machines and workers.
- Whether Windows and Android default selections feel useful.
- The prominence of decision requests and production incidents.
- The proposed Machines / Workers navigation and provisional name/mark.

The next coherent screen after approval is the worker/task detail, including requests for a decision and the distinction between observing a task and controlling it.

## Source and verification

These fragments use inline fixture data and no application API. They are independent of Flutter/Go. Preserve the current product specification as the source of accepted requirements. Visual acceptance does not approve unimplemented backend contracts or finalize the product name.

`tokens.json` is the source for the three Flutter color palettes, generated through `packages/prisme_ui/tool/sync_tokens.dart`. Flutter spacing, typography, density and native layout rules are documented in the package. The teal option remains exploratory; cobalt is the documented baseline.

The matching conversation copies live in the task's visualization directory. The original v0.1 inline views remain a record of the accepted baseline. Run `refresh_previews.py <inline-directory>` after changing `home.html` to regenerate the current mobile and Matrix views from their shared source.

## v0.2 — mobile preview and Terminal / Matrix

The user accepted the initial direction and layout, and requested two additions: an immediately visible mobile home preview, and a selectable terminal/Matrix theme with the same layout.

- [home-mobile.html](home-mobile.html) opens directly on Android in the classic dark theme, with its own device selection and persistent bottom navigation.
- [home-matrix.html](home-matrix.html) opens on Windows in the new Matrix theme.
- Both views allow switching between the system appearance, light, dark, and Matrix. The selector is available in **Personnaliser → Apparence**, as well as the review controls. Apply commits the local choice; closing the panel discards it.
- Matrix uses nearly black green-tinted surfaces, phosphor-green active elements, IBM Plex Mono interface typography, restrained glow on metrics, and faint static scanlines. There is no animated rain or flashing content.
- Amber still means attention; red still means incident. Muted, explicitly labelled stale data remains distinguishable from an active connection. Action labels, icons, keyboard focus, content order, spacing, card shapes, and grid structure are shared with the accepted layout.
- The classic cobalt/teal accent preference is retained when entering Matrix and restored on leaving it. Matrix has its own fixed phosphor palette and uses dark form controls regardless of the host appearance.
- The accepted Matrix theme is recorded under `color.matrix` in `tokens.json`. It does not alter the existing light/dark token values.

### Vérifications de la v0.2

- Aperçu Android direct vérifié à 412 px ; thème Matrix vérifié à 360 px sur mobile et à 1 024 px sur desktop, sans débordement horizontal détecté.
- Bascule Matrix → Clair et Sombre → Matrix vérifiée depuis Personnaliser ; les sélections de ressources et la structure de mise en page sont conservées.
- Les 16 paires texte/fond du thème Matrix dépassent 4,5:1 ; le minimum mesuré est de 6,29:1. Il s’agit d’une vérification de palette, pas d’un audit complet d’accessibilité.
- Syntaxe JavaScript validée pour la source commune et ses deux aperçus dérivés.

### Verification completed for v0.1

- JavaScript syntax checked for both fragments.
- Visually inspected the foundations, dark Windows incident view at 1,024 px content width, and light Android views at 412 px and 360 px.
- No horizontal content overflow detected in the inspected desktop and narrow mobile views.
- Checked personalization Apply, search filtering, and worker preview with old-data context.
- Twenty-five baseline text/background pairs passed 4.5:1; the lowest measured contrast was 4.99:1. This is a palette check, not a full accessibility audit.
- The native Android and Windows implementations, all scenario combinations, and the Codex-hosted design-control bridge still require their own validation.
