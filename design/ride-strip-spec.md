# Ride Status Strip — Design Spec

**Component:** `RideStrip.svelte` · **Mode:** bike ride mode
**Placement:** below the map (desktop/tablet); bottom-sheet peek header on phone

Tags: **[DOM]** = established domain research · **[REF]** = external reference · **[DJ]** = design judgment.

> **CORRECTION APPLIED:** the source spec said bump schema v19 → v20. The real
> `currentSchemaVersion` in `internal/store/sqlite.go` is **24**, and the
> in-flight backend workflow (WP1–WP5) is already bumping it. Take the schema
> version from the tree at implementation time; do not hardcode from this doc.

> **SCOPE NOTE:** the backend types this spec names (`Shutoff`, `SweepReport`,
> `SagRequest`, `SagVehicle`, `PendingReply`, `Shift`) are being built by the
> backend workflow as WP2–WP5. `Phase` / `RidePhase` was NOT in that scope and
> is tracked separately as **WP5b**. Build the frontend against the API surface
> the backend Verify stage emits, not against the type sketches here.

---

## 0. Governing decision: separate component, shared stores

**Do not extend `SituationBoard.svelte`. Do not replace it. Build `RideStrip.svelte` alongside it.** [DJ]

1. **Axis.** SituationBoard is a vertical stack sized for the 480px `--panel-width` column. The strip is a horizontal band with a hard height contract. No shared layout to factor out.
2. **Lifecycle.** SituationBoard mounts only inside `NetControlPanel.svelte`'s `currentTab === 'situation'`. It is *not always visible* — which is the strip's entire premise.
3. **Relationship.** SituationBoard is already "everything needing attention, in a list." That is the strip's *overflow destination*. One component would make the relationship circular.

**SituationBoard is the strip's detail view.** They share derived stores, never markup. Every strip zone is a navigation target.

Follow-up: SituationBoard renders `<RouteProgressBar>`. Suppress that copy when `isDesktop && rideMode` and let the strip own the rail.

---

## 1. Wireframes

### Desktop (>=1200px wide, >=500px tall) — 128px, two rows

```
┌──────────────────────────────────────── MAP (leaflet) ────────────────────────────────────────────────┐
│                                                                                                        │
└────────────────────────────────────────────────────────────────────────────────────────────────────────┘
╔════════════════════════════════════════════════════════════════════════════════════════════════════════╗
║ ROW A — COURSE RAIL                                                                            60px    ║
║        ▼LEAD 68.2 mi                                                                                   ║
║  100mi ●━━━━━━◆RS1━━━━━━◆RS2━━━━╫BENSON━━━◆RS3━━━━━━◇RS4━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━●FIN    ║
║               ⊘       ⏳                11:00                                                          ║
║        ▲SWEEP 31.4 mi · 11.4 mph · 6m ago          gap 36.8 mi / ~2h10m          22 riders back        ║
╟────────────────────────────────────────────────────────────────────────────────────────────────────────╢
║ ROW B — ZONES                                                                                  66px    ║
║ ┌────────┬──────────────┬──────────────┬─────────────┬──────────────┬─────────────┬────────┬────────┐ ║
║ │ NET    │ LAST RIDER   │ NEXT SHUTOFF │ STOPS       │ SAG          │ TRAFFIC     │ WX     │ SHIFT  │ ║
║ │ 4:12   │ ▲ mi 31.4    │ ╫ BENSON     │ ◆4 ⊘2 ⏳1   │ ⬒ 2 open     │ ◆PR 1  △HI 2│ 94°F   │KE7ABC  │ ║
║ │ MID-   │ 31 to go     │ 11:00 −0:47  │ RS2 awaits  │ 3 riders 17m │ ! 2 unacked │ HI 101°│ −0:23  │ ║
║ │ RIDE   │ 11.4mph · 6m │ 22 riders bk │ sweep       │ seats 6/9    │ ⏱ 1 pending │ ⚠ Heat │ ⇄ brief│ ║
║ └────────┴──────────────┴──────────────┴─────────────┴──────────────┴─────────────┴────────┴────────┘ ║
║   120px      200px          180px          160px         180px          190px       110px    120px    ║
╚════════════════════════════════════════════════════════════════════════════════════════════════════════╝
```

**Rail is Row A, adjacent to the map.** The rail is the map's x-axis; the eye travels straight down from a dot on the map to its mile marker. Tiles have no spatial relationship to the map and belong further away. [DJ]

**Below the map, not above.** The top is claimed by `WxInterruptBanner`, `GpsStatusPill`, `WxLinkPill`, `NextStopPill`, the ops-view FAB. More importantly the *interrupt* channel must stay top and unobstructed — the strip must never sit between the operator and an emergency banner. [REF: dark-cockpit / master-warning]

### EMERGENCY state — the strip does not grow

```
╔══ 4px solid var(--color-error) ════════════════════════════════════════════════════════════════════════╗
║ ROW A — rail unchanged, incident pin ⬣ drawn at mi 44.1                                                ║
╟────────────────────────────────────────────────────────────────────────────────────────────────────────╢
║ ┌────────┬──────────────┬──────────────┬─────────────┬──────────────────────────────────────┬────────┐ ║
║ │ NET    │ LAST RIDER   │ NEXT SHUTOFF │ STOPS       │ TRAFFIC                              │ SHIFT  │ ║
║ │ 4:12   │ ▲ mi 31.4    │ ╫ BENSON     │ ◆4 ⊘2 ⏳1   │ ⬣ EM  Rider down, mi 44.1  ·  0:03   │KE7ABC  │ ║
║ │ MID-   │ 31 to go     │ 11:00 −0:47  │ RS2 awaits  │ RS3 reporting · ambulance requested  │ −0:23  │ ║
║ │ RIDE   │ 11.4mph · 6m │ 22 riders bk │ sweep       │ +2 others          [ Acknowledge a ] │ ⇄ brief│ ║
║ └────────┴──────────────┴──────────────┴─────────────┴──────────────────────────────────────┴────────┘ ║
╚════════════════════════════════════════════════════════════════════════════════════════════════════════╝
   SAG and WX zones are ABSORBED, not stacked. Height unchanged. Full incident lives in the
   InterruptBanner above the map; this is the persistent latch.
```

### Tablet (769–1199px) — 140px, six zones

```
╔════════════════════════════════════════════════════════════════════════════════╗
║        ▼LEAD 68.2                                                               ║
║  ●━━━━◆1━━━━◆2━━╫━━━◆3━━━━◇4━━━━━━━━━━━━━━━━━━━━━━━━━━━━●    (labels on focus) ║
║        ▲SWEEP 31.4 · gap 36.8mi/2h10m · 22 back                                 ║
╟─────────────────────────────────────────────────────────────────────────────────╢
║ ┌───────────┬──────────────┬─────────────┬───────────────┬───────────┬────────┐ ║
║ │NET+SHIFT  │ LAST RIDER   │NEXT SHUTOFF │ LOGISTICS     │ TRAFFIC   │ WX     │ ║
║ │4:12 MID   │ ▲ mi 31.4    │╫ BENSON     │ ◆4 ⊘2 ⏳1     │◆PR1 △HI2  │ ⚠ 101° │ ║
║ │KE7ABC −23 │ 31 to go     │11:00 −0:47  │ ⬒ 2 SAG 3 rdr │! 2 unack  │        │ ║
║ │⇄ brief    │ 11.4mph · 6m │22 riders bk │ seats 6/9     │⏱ 1 pend   │        │ ║
║ └───────────┴──────────────┴─────────────┴───────────────┴───────────┴────────┘ ║
╚═════════════════════════════════════════════════════════════════════════════════╝
```

Merges: NET+SHIFT; STOPS+SAG -> LOGISTICS; WX -> glyph + heat index only.

### Phone (<769px) — not a strip

A 44px line inside `BottomSheet`'s `peekContent`, by the exact mechanism `wx-peek-strip` already uses (`peekExtraH`, `--sheet-peek` published at runtime).

```
┌─────────────────────────────────────┐
│              MAP                    │
├─────────────────────────────────────┤  <- bottom sheet, 'peek'
│ ━━━━  (drag handle)                 │
│ ● Connected     14 on roster        │
│ ┃▲ mi 31.4 ╫ −0:47  ◆PR1 △2    ›   │  <- RIDE PEEK (44px, new)
│ ┃  (left border = highest tier)     │
│ ⚠ Heat Advisory        1h 12m   ›   │  <- existing wx-peek-strip
│ [🔍][✉][📍][⚑][☁][⚙] ...            │  <- Toolbar
└─────────────────────────────────────┘
```

Tapping opens `NetControlPanel` -> `situation` tab, where the content renders **vertically**, not as a squeezed strip.

Landscape phone (`max-height: 499px`): ride peek **suppressed entirely**, same media query as `.desktop-only`. [DJ]

---

## 2. Zone table

| # | Zone | Content | Data source | Rank | Empty / unknown / stale |
|---|------|---------|-------------|------|--------------------------|
| A | **Course rail** (full width) | Route line; stop nodes by sequence; shutoff gates `╫`; LEAD **above** the line; SWEEP **below**; incident pins | `checkpoint.CheckpointWithPassages`, `store.CheckpointMeta.SequenceNumber`, `ProgressElement.Label/LastCheckpointSeq`; miles via `$lib/routeDistance` `projectOnRoute`/`courseDistance` against the `route` Annotation. Shutoffs **NEW** | **1** | **No course:** one line — "No course loaded — import GPX to enable ride mode" + button; strip drops to single 66px row. **No passages:** rail drawn, all nodes `◇ planned`, "No lead or sweep reported." **Lead only:** dashed ghost at last-known + literal `SWEEP — not reported`, never `0`. |
| B1 | **NET** | Elapsed `4:12`; phase chip | `store.Net.OpenedAt`; phase **NEW** | 6 | Not open: `—` + "No net open"; chip reads `PRE-START`. |
| B2 | **LAST RIDER** | `▲ mi 31.4` · `31 to go` · `11.4 mph · 6m ago` · `22 riders back` | Sweep mile derived from `ProgressElement`; speed + accumulator **NEW** (`SweepReport`) | **2** | Never: `▲ —` + `SWEEP not reported` muted. >20m: value muted, age chip amber, literal **stale**. Never green. |
| B3 | **NEXT SHUTOFF** | `╫ BENSON` · `11:00  −0:47` · `22 riders back of it` | **NEW** `Shutoff{Name, Lat, Lon, CloseAt, DivertTo, RouteIDs, FiredAt, FiredBy}` | **3** | None configured: zone **hidden**. All fired: `╫ all shutoffs fired` in `--color-success`. |
| B4 | **STOPS** | `◆4 ⊘2 ⏳1` + name of the one blocking closure | `annotation.Category == "checkpoint"` + `Status`. "Awaiting sweep" **NEW** status | **4** | Zero stops: hidden. All closed: `⊘ all 9 closed` success. |
| B5 | **SAG** | `⬒ 2 open` · `3 riders · 17m` (oldest) · `seats 6/9 · racks 4/9` | **NEW** `SagRequest`, `SagVehicle`; vehicles cross-ref `NetCheckIn.Category == "sag"` | **5** | No open: `⬒ 0 open` muted + seats. **No SAG vehicles checked in: `⬒ no SAG units` in `--color-warning`** — an absence that matters. |
| B6 | **TRAFFIC** | Tier glyph+count for each tier with count>0; `! N unacked`; `⏱ N pending` | `NetMission.Priority`/`Status` (extend enum to 5 tiers) or incident Annotations; unacked from `NetCheckIn.Traffic`; pending **NEW** | **2** | Nothing open: single `✓ clear` **muted, not green** — green means verified good; this is just nothing logged. [REF: wall-of-green anti-pattern] |
| B7 | **WX** | Temp · heat index · highest in-area alert glyph | `store.WeatherReading`; heat index **NEW** (derived, backend); alerts via `wxInAreaAlerts` + `WxTierGlyph` | 7 | No station and no alert: **hidden**. Temp only: single line `94°F`. Conditional rendering is the point. |
| B8 | **SHIFT** | NCS callsign · `−0:23` to relief · `⇄ brief` | `store.Net.NCSCallsign`; shift **NEW** | 6 | No shift configured: callsign only, no countdown. |

### Rejected / reshaped

| Candidate | Verdict | Why |
|---|---|---|
| Riders still on course | **Reshaped** -> `22 riders back` inside B2 | No roster, so this is only an NCS-held accumulator [DOM]. Meaningless without the sweep it hangs off. |
| Lead rider position | **Rail only, no tile** | Lead matters as *events* ("first participant arrival" is HIGH [DOM]), not a standing number. |
| Sweep position + gap | **Accepted — largest tile** | "The position of the last participant is one of the most frequently asked questions" [DOM]. Gap in **both** distance and time; the radio question comes both ways. |
| Open SAG by status | **Reshaped** -> open count + riders waiting + oldest age | Five buckets is five numbers. What changes a decision is how many humans are at the roadside and for how long. |
| SAG seats/racks | **Accepted, one fraction each** | Hard constraint on whether a request can be filled. |
| Rest stops open/closed/at-capacity | **Reshaped** | "How many open" changes nothing. "Which one can't close yet" does [DOM]. At-capacity is an *incident* -> B6. |
| Next shutoff + countdown | **Accepted — top 3** | A wall-clock deadline that forcibly re-routes people [DOM]. |
| Active incidents by tier | **Accepted, dark-cockpit** | Only tiers with count>0 render. Five permanent zero-counters is the wall-of-green failure. |
| Unacknowledged traffic | **Accepted, one number** | Existing `NetCheckIn.Traffic`. |
| Stations checked in / missing / overdue | **"Checked in" rejected; overdue folded into B6** | Standing roster count never changes a decision mid-ride. **Exception:** in PRE-START it is the only thing that matters — see §7. |
| Elapsed net time | **Accepted, tiny** | Denominator for every "how long since" question [DOM]. |
| Temp + HI + WX alert | **Accepted, conditional** | Hidden when unremarkable; HI promotes the zone above a configured threshold. |
| Time since last contact per unit | **Rejected as enumeration** | 25 units x a timestamp is a roster. Aggregate in B6; the list is the roster tab sorted by `lastHeard`. |

---

## 3. Color, glyph and token contract

**Rule: no new hues.** Alias the semantic trio exactly as `--color-wx-*` does.

```css
/* Ride priority tiers (Marin ARS five-tier taxonomy). Deliberately NOT new
   hues — aliases of the semantic trio, same discipline as --color-wx-*.
   RED IS EXACTLY ONE TIER. PRIORITY and HIGH share amber and are told apart
   by GLYPH (filled vs open triangle) and by the two-letter code. */
--color-ride-emergency: var(--color-error);
--color-ride-priority:  var(--color-warning);
--color-ride-high:      var(--color-warning);
--color-ride-medium:    var(--color-info);
--color-ride-low:       var(--color-text-muted);

--color-ride-lead:  var(--color-success);
--color-ride-sweep: var(--color-accent);

/* Strip geometry. Published to :root at runtime by RideStrip.svelte, the same
   way BottomSheet.svelte publishes --sheet-peek, so .map-layer's inset and
   Leaflet's invalidateSize() can never drift from the rendered height. */
--ride-strip-h: 128px;
```

**Type scale.** `app.css` has spacing/radius/shadow/z-index/animation tokens but **no type scale**; components hard-code `0.8rem` etc. The strip needs one across 8 zones. Name for role, not size:

```css
--ride-t-value: 1.375rem;  /* the number you read across the room */
--ride-t-body:  0.875rem;
--ride-t-label: 0.6875rem; /* zone headers, uppercase, letter-spacing .06em */
```

Everything else uses existing tokens: `--space-xs/sm/md`, `--radius-sm`, `--color-surface`, `--color-bg`, `--shadow-md`, `--duration-fast`, `--ease-out`, `--z-toolbar`.

### Contrast (computed against `--color-surface` #16213e)

| Token | Ratio | Verdict |
|---|---|---|
| `--color-text` #eee | ~15.5:1 | pass |
| `--color-text-muted` #aaa | ~7.4:1 | pass |
| `--color-warning` #f59e0b | 7.41:1 | pass as text |
| `--color-success` #4ade80 | 9.15:1 | pass as text |
| `--color-error` #e74c3c | **4.16:1** | **fails AA for normal text** |
| `--color-info` #3b82f6 | **4.32:1** | **fails AA for normal text** |

**Load-bearing consequence:** `--color-error` and `--color-info` may be used for **glyph fills, borders and rules** (>=3:1 non-text — both pass) but **never for a numeral or label at body size**. EMERGENCY and MEDIUM chips render text in `--color-text` and carry tier via glyph + 3px left border + two-letter code. Same conclusion `app.css` already reached for `--color-wx-statement`. [DJ, computed]

### Glyph vocabulary

New `web/src/lib/rideMeta.ts`, same shape as `wxAlertMeta.ts` — `{label, colorVar, softVar, glyph, glyphFill, ariaWord, code}` — consumed by `RideTierGlyph.svelte`, a near-copy of `WxTierGlyph.svelte` including the punch-out-in-`--color-bg` trick.

| Thing | Glyph | Code | Non-color channel |
|---|---|---|---|
| EMERGENCY | filled octagon, `!` punched in `--color-bg` | `EM` | unique shape + code + full-width top border |
| PRIORITY | filled triangle, `!` punched | `PR` | filled vs HIGH's open |
| HIGH | open triangle | `HI` | |
| MEDIUM | open diamond | `MD` | |
| LOW | open circle | `LO` | |
| LEAD | filled chevron **above** the rail | — | **side of the rail** + always-present text label |
| SWEEP | filled chevron **below** the rail | — | **side of the rail** + always-present text label |
| Stop: planned | `◇` | | |
| Stop: open/active | `◆` | | |
| Stop: awaiting sweep | `⏳` filled diamond + bar | | |
| Stop: at capacity | `⊛` filled diamond + ring | | |
| Stop: closed | `⊘` | | |
| Shutoff armed | `╫` gate across the rail | | reads as a barrier; unmistakable in grayscale |
| Shutoff fired | `╫` with rail behind it dashed | | |

Lead-vs-sweep **by side of the rail** is the key accessibility move: survives grayscale, color-blindness, and glare on an outdoor laptop. [REF: RAG status]

---

## 4. Alerting escalation matrix

**Premise: the strip is the Notify tier, never the interrupt.** [REF: Datadog Record/Notify/Page]. The interrupt channel is a single full-width banner above the map, and there must be exactly one in the app.

| Tier | Strip treatment | Motion | Sound | aria-live | Persistence | Dismissal |
|---|---|---|---|---|---|---|
| **EMERGENCY** | 4px `--color-error` border across the strip's top edge. B6 expands, absorbing SAG+WX, showing the incident's label, location, elapsed. Other zones drop to 60% opacity for 3s then return. | **One** 2s attention pulse of the border on arrival (3 cycles), then dead steady. Nothing on the strip blinks continuously. Killed by the global `prefers-reduced-motion` rule. | Interrupt banner's 3x880Hz chime + `navigator.vibrate`, **repeating every 60s until acknowledged** | `assertive` **in the banner only**, never the strip | **Latched.** Never auto-clears. Survives reload and shift change. | Explicit `Acknowledge` (logs who + when to `NetEvent`), then downgrades to a PRIORITY-styled chip until resolved. |
| **PRIORITY** | Glyph + count + oldest age in B6; 3px left border | none | Single chime once, only if sounds enabled | `polite` | Until resolved/downgraded | `a` acks; resolution is a panel action |
| **HIGH** | Glyph + count | none | none | `polite`, **batched to one per 60s** | Until resolved | — |
| **MEDIUM** | Glyph + count | none | none | none | Until resolved | — |
| **LOW** | Rendered only if nothing higher present; else folded into `+N` | none | none | none | Until resolved | — |

### Degradation when three things are wrong at once

1. **The strip never grows a row.** 128px is a contract. Growth is how a status board becomes a scroll region and stops being glanceable.
2. **One escalation owns the interrupt channel; the rest queue.** Reuse `WxInterruptBanner`'s existing `wxInterruptQueue`/`wxInterruptMore` machinery, which already renders "+N more" — generalize into `InterruptBanner.svelte`, make `WxInterruptBanner` a thin wrapper. **Do not build a second banner.** Two competing full-width banners *is* the christmas tree.
3. **The top border shows the single highest tier only.** Never striped, never split.
4. **B6 shows the highest tier full size + `+2` muted.** Per-tier counts render only where count>0.
5. **The desaturation funnel fires at most once per 5 minutes** regardless of arrival rate, so a burst cannot strobe.
6. **Hysteresis on auto-derived conditions only.** A machine-derived condition (unit overdue, WX threshold crossed, stop at capacity) must be stable 30s before it can raise the strip's top tier. A human-logged incident escalates instantly — the operator already applied judgment. [REF: Turkish Airlines 1951]
7. **Sound budget: at most one audible event per 30s app-wide.** EMERGENCY always wins arbitration. [REF: conservative adaptation of EEMUA 191 / ISA 18.2 practice — the commonly quoted numeric thresholds are paywalled and were NOT independently verified.]
8. **Never auto-resolve a red silently.** Auto-clearing destroys trust and the audit trail ICS-214 depends on.

---

## 5. Actions — what is clickable

**The strip is read-only for state changes, with exactly four exceptions.** [DJ]

Every commit button costs a 44px target and invites a misclick at the worst moment; but routing a time-critical action through three panel clicks is its own failure. Resolution: **multi-field records go through the command palette via a keyboard accelerator** — the codebase is already built for this (`commandParser.ts` already parses `cp3 lead`, `mission <title>`, `<callsign> <status>`).

| Affordance | Behavior | Why it earns the exception |
|---|---|---|
| **Every zone is a navigation button** | Opens the relevant panel tab / SituationBoard section / flies the map. Mutates nothing. | Navigation is free and reversible. This is how the strip stays thin. |
| **`Acknowledge` in B6** | Commits immediately, logs actor + timestamp | Acking's whole value is being instant; logged and reversible; mirrors `WxInterruptBanner`'s existing Acknowledge, so operators already know it. |
| **`Passed` on the sweep marker** | Opens a **one-field** confirm (which checkpoint); does not commit blind | A wrong sweep passage corrupts the most-asked-about number in the operation [DOM]. One confirm click is cheap insurance. |
| **Phase chip** | Opens the phase-change confirm | Phase is never switched silently (§7). |

**Explicitly NOT clickable-to-commit:** every counter. Counters navigate, never mutate. Logging a SAG, creating an incident, closing a stop are multi-field records and go through the palette.

### Keyboard shortcuts

Inert while focus is in `input`/`textarea`/`[contenteditable]`, while `LoginOverlay` or `SetupWizard` is up, and while the command palette is open. Registered in `+page.svelte`'s existing `handleGlobalKeydown`.

| Key | Action |
|---|---|
| `Ctrl`/`Cmd`+`K` | Command palette *(existing)* |
| `/` | Search *(existing)* |
| `e` | Palette pre-seeded and **locked to EMERGENCY**, composer focused. One key to get there; `Enter` commits. Never fires blind. |
| `n` | Palette pre-seeded `incident ` (defaults PRIORITY) |
| `s` | Palette pre-seeded `sag ` |
| `q` | Palette pre-seeded `lead ` |
| `w` | Palette pre-seeded `sweep ` |
| `c` | Palette pre-seeded `close ` |
| `a` | **Acknowledge the top unacknowledged item.** Commits. |
| `r` | Open pending-replies list |
| `x` | Open the shift relief briefing |
| `?` | Shortcut help overlay |
| `Esc` | Cancel / close *(existing convention)* |

`q`/`w` is a **positional** mnemonic: `q` is left of `w` exactly as LEAD is left of SWEEP on the rail. [DJ]

`s`/`c`/`l`/`n`/`p` are also single-key handlers *inside* `CommandPalette.svelte`, but palette-scoped and never simultaneously reachable. No letter was reused with a *different* meaning across the two scopes.

---

## 6. Time and staleness

**Deadlines (shutoffs, shift relief).** Always show **absolute wall-clock**; add a countdown inside 90 minutes. Never both at second resolution.
`╫ BENSON  11:00  −0:47`
Absolute is checkable against a wristwatch and survives looking away; the countdown drives the decision. [REF: NNGroup on false precision]

**Minute resolution everywhere except the net clock.** `sweep ETA 14:32:07` is a lie with a decimal point.

**A fired shutoff flips state discretely and loudly** — glyph changes, rail behind it goes dashed, a `NetEvent` is written. It does not fade out. [REF: Comrades Marathon fires a gun at exactly 12:00:00]

**Ages.** Relative text at a glance, absolute in `title`. [REF: github/relative-time-element]

Three named states, not a gradient — a gradient can't be read at a glance or described on the radio:

| State | Threshold | Rendering |
|---|---|---|
| `fresh` | < 10m | `--color-text`, plain `6m` |
| `aging` | 10–20m | `--color-warning` age chip, `17m` |
| `stale` | > 20m (reuse `STALE_THRESHOLD_MS` from `netcontrol.ts`) | muted value + the **literal word `stale`**. Never color alone. |
| `never` | no report ever | `—` + `not reported`. **Visually distinct from `stale`.** |

The `never` state is non-negotiable and is the most-copied idea from the research. [REF: Grafana's explicit **NoData** state, separate from Normal and Alerting]. A never-reported value must never render as `0`; a stale value must never be painted green.

**Ticking.** Do not spawn a second interval. `stores/wxAlerts.ts` already exports `wxClock`/`wxMinute`; **promote to `$lib/stores/clock.ts` as `secondClock`/`minuteClock`** and re-export from `wxAlerts.ts` for compatibility. Without a shared ticking store a "6m ago" chip silently drifts for the whole shift. Reuse `wxAlertTime.ts`'s `countdown()`, `countdownTone()`, `clock()`, `clockWithDay()` unchanged — they already take `now` as an explicit parameter.

**The elapsed-time question is answered by the log, not the strip.** The strip shows the age of the *current* state; "how long since the ice truck said 20 minutes" is a query against the pending-replies list and the timeline. The strip's job is to make you notice it's been 43 minutes.

---

## 7. Shift change — the PENDING element

**Yes, the strip carries a pending/awaiting-reply element.** B6's third line: `⏱ 3 pending · oldest 22m`.

This is the highest-value new data type in the spec: the one thing the briefing checklist names explicitly ("messages sent and replies you expect, and who gets the reply") and the one thing absent from the current data model. [DOM]

```go
// internal/ride — NEW
type PendingReply struct {
    ID          string     `json:"id"`
    NetID       string     `json:"netId"`
    SentTo      string     `json:"sentTo"`       // tactical call or unit
    Subject     string     `json:"subject"`      // "ice truck ETA"
    SentAt      time.Time  `json:"sentAt"`
    SentBy      string     `json:"sentBy"`
    ExpectBy    *time.Time `json:"expectBy,omitempty"` // "20 minutes out" -> now+20m
    OwnerUserID string     `json:"ownerUserId"`  // WHO GETS THE REPLY
    ClosedAt    *time.Time `json:"closedAt,omitempty"`
    ClosedNote  string     `json:"closedNote"`
}
```

A pending item past `ExpectBy` promotes its age chip to `--color-warning`; at 2x the expected interval it raises B6 to MEDIUM. That is the direct answer to "how long since the ice truck said 20 minutes out?" — it turns a memory question into a visible number.

**The briefing itself** (`x`, or `⇄ brief` in B8) opens `ShiftBriefing.svelte`, deliberately *not* a strip element:

1. **A frozen snapshot** of every strip value at the moment it opened — the ICS-201 equivalent. [REF: ICS 201 Incident Briefing (snapshot) vs ICS 214 Activity Log (chronology) are architecturally different artifacts and must not be collapsed.]
2. **The full pending list**, grouped by `OwnerUserID`.
3. **Open items by tier**, with elapsed.
4. **A read-back confirm**: the incoming operator presses `I have the net`, writing an `ncs_transfer` `NetEvent` (type and icon already exist in `SituationBoard.svelte`'s `eventIcons`). B8 does not change hands until that button is pressed.

The read-back is the deliberate steal. [REF: I-PASS's distinguishing element vs SBAR is **Synthesis by receiver** — the incoming person restates before taking over. AHRQ PSNet notes results on SBAR *alone* are mixed: structure without read-back is not enough. Exact Starmer et al. NEJM 2014 effect sizes were NOT independently verified.]

The `−0:23` countdown starts amber at T−15 and shows `⇄ brief` as a call to action. It does not escalate beyond that — a shift change is not an emergency.

---

## 8. Phase model  *(tracked as WP5b — not in the WP1–WP5 backend scope)*

**Content shifts by phase.** Seven named phases collapse to six UI states; the zone *set* changes, the height never does.

**Phase is operator-set, never silently auto-switched.** The system *suggests* (`⟳ CLOSING?` on the chip, one pulse) and the operator confirms. Silent state change is the classic ICS failure of an assignment nobody logged. [DJ]

| Phase | Trigger (suggested) | Rail | Zone set | What changes |
|---|---|---|---|---|
| **PRE-START** | net opened, no passages | Stops + shutoffs only, no edge markers | NET · **CHECKED IN** · STOPS · WX · SHIFT | The *only* phase where roster count earns a zone (`14 in · 2 missing`). SAG and TRAFFIC dark. |
| **LAUNCHED** | first lead passage | LEAD live, SWEEP ghosted at start | NET · **LEAD** · STOPS · SAG · TRAFFIC · WX · SHIFT | LEAD gets the big tile; "first arrival at RS_n_" fires HIGH at each stop. |
| **MID-RIDE** | sweep passes stop 1 | Full | All 8 as drawn | Steady state. LAST RIDER takes the big tile permanently. |
| **CLOSING** | T−60 to first shutoff | Armed gates highlighted | NEXT SHUTOFF **doubles width**, shows next *two* | New readout: **`riders at risk`** = accumulator between the armed shutoff and the sweep. Drives the divert decision. |
| **COLLAPSE** | NCS declares, usually after lead finishes | LEAD lane **retires**; its space becomes a stops-cleared fill from the back forward | NET · LAST RIDER · **CLEARING** · SAG · TRAFFIC · SHIFT | STOPS becomes CLEARING: `6 of 9 cleared · waiting on RS7 · 1h04m`. WX drops unless an alert is active. The rolling back-to-front collapse [DOM] is literally the rail filling in from the left. |
| **RECONCILE** | last stop clear | Rail collapses to a 20px summary line; strip drops to ~90px | **UNACCOUNTED** · SAG'D TOTAL · OPEN INCIDENTS · UNITS NOT RELEASED · SHIFT | The closeout checklist. Every tile should reach zero before the net closes. |

---

## 9. Responsive breakpoints

Reuse the existing queries verbatim — do not invent a third breakpoint system. `app.css` establishes `(max-width: 768px), (max-height: 499px)` = mobile and `(min-width: 769px) and (min-height: 500px)` = desktop, kept in sync with `+page.svelte`'s `matchMedia` and `BottomSheet.svelte`'s `SHORT_VH_BREAKPOINT`.

| Viewport | Treatment |
|---|---|
| **>=1200w, >=500h** | Full 2-row strip, `--ride-strip-h: 128px`, 8 zones. |
| **769–1199w, >=500h** | 2-row, `140px` (tiles need a third line), 6 zones: NET+SHIFT merged, STOPS+SAG -> LOGISTICS, WX -> glyph + HI. Rail keeps full width; stop labels become sequence numbers, full names on hover/focus. |
| **<=768w** | Not a strip. 44px `ride-peek` inside `BottomSheet`'s `peekContent`, above `wx-peek-strip`, budgeted via `peekExtraH` (`RIDE_PEEK_STRIP_H = 44`, mirroring `WX_PEEK_STRIP_H = 36`). **Exactly three facts:** sweep mile, next shutoff countdown, highest open tier + count. Everything else is one tap into the `situation` tab, rendered **vertically**. |
| **<=499h (landscape phone)** | Suppressed entirely. Same query as `.desktop-only`. |

### Required layout change

`.map-layer` is currently `position: absolute; inset: 0`. The strip must not overlay the map (it would cover the course it describes):

```css
.map-layer { inset: 0 0 var(--ride-strip-h, 0px) 0; }
```

`RideStrip.svelte` publishes `--ride-strip-h` on `:root` at mount and cleans up on destroy — **exactly** the recipe `BottomSheet.svelte` uses for `--sheet-peek`, so inset and rendered height can never drift. `MapPalette`'s FAB already lifts off `--sheet-peek`; it needs the same for `--ride-strip-h`.

**Leaflet must be told.** Call `map.invalidateSize()` on strip mount, unmount, and any height change (phase transition into RECONCILE). Omitting this leaves Leaflet with a stale container height and every click offset by 128px — a silent, maddening bug.

---

## 10. Accessibility

**Contrast.** See §3 — `--color-error` and `--color-info` are confined to glyphs, borders and rules; their tier text renders in `--color-text`.

**No status encoded in color alone.** Every state has at least two of {shape, position, text}:

| State | Color | Shape | Text |
|---|---|---|---|
| Tier | aliased token | unique glyph | two-letter code `EM/PR/HI/MD/LO` |
| Lead vs sweep | success / accent | chevron | **side of the rail** + literal `LEAD`/`SWEEP` |
| Stop status | `statusMeta` colors (existing) | `◇ ◆ ⏳ ⊛ ⊘` | full status word on focus |
| Shutoff armed vs fired | — | gate glyph; rail dashed once fired | `11:00` vs `FIRED 11:00` |
| Staleness | muted | — | the literal word **`stale`** |
| Never reported | muted | `—` | `not reported` |

**Keyboard.** The strip is a **single tab stop** with roving `tabindex` and arrow-key navigation across zones (WAI-ARIA toolbar pattern). Tabbing out of the map must not walk the operator through thirty chips. `Home`/`End` jump to first/last zone. Within the rail, `←`/`→` move between stop nodes, `Enter` flies the map to the focused stop.

**Screen reader.**
- Strip: `<section role="region" aria-label="Ride status">`.
- Each zone: `role="group"` with `aria-labelledby` pointing at its label span.
- Every glyph: `<svg role="img"><title>…</title></svg>`, as `WxTierGlyph.svelte` already does.
- **One** `aria-live="polite"` region, carrying tier transitions and shutoff arming, batched to at most one announcement per 60s.
- **Never `assertive`.** Assertive belongs solely to the interrupt banner, which already moves focus to its Acknowledge button. A strip that interrupts a screen-reader user on every counter tick is unusable within ten minutes.
- Numbers get a spoken form: `aria-label="Sweep at mile 31.4, 31 miles remaining, 11.4 miles per hour, reported 6 minutes ago"` rather than letting the reader spell out `▲ mi 31.4 · 31 to go`.

**Motion.** The only animation in the strip is the single EMERGENCY arrival pulse; the global `prefers-reduced-motion` rule already reduces it to 0.01ms. Nothing else moves, ever.

**Outdoor legibility** (a laptop in a parking lot in July): `--ride-t-value` at 1.375rem and the glyph-plus-code redundancy are both sunlight measures. The existing `--wx-casing` halo is the precedent for map strokes.

---

## 11. Anti-recommendations — what does NOT go in the strip

| Rejected | Belongs in |
|---|---|
| Any scrolling list or feed | `ActivityPanel` / SituationBoard timeline. You cannot glance at something you have to scroll. |
| Individual bib numbers | The SAG request record and the exception log. Bibs are labels, not keys [DOM]; a bib on the strip implies a roster that does not exist. |
| Full station/roster list | `NetControlPanel` roster tab (already there, with `netMetrics`). |
| Net name, frequency, NCS full name | A header or the net panel. Static text never changes a decision. |
| Transport/packet connection state | `ConnectionStatus`, `WxLinkPill`, `GpsStatusPill`. Duplicating it gets you two disagreeing indicators. |
| Message previews / convo list | `ConvoList`. A count of unacked traffic yes; message text no. |
| A map legend | The map. |
| Mission/assignment list | Missions tab. Count only. |
| Per-tier zero counters | Nothing — render only when >0. |
| A second clock (ride elapsed beside net elapsed) | One clock. Two clocks in a status band get misread under stress. |
| Sparklines / charts | `WeatherChart` / `TelemetryPanel`. A trend line in a 66px tile is decoration. |
| SAG'd rider running total, mid-ride | RECONCILE phase only. Mid-ride nobody acts on it. |
| "System healthy" / "all clear" green badges | Nothing. Absence is the signal. |
| Anything requiring hover to be *readable* (vs *detailed*) | The strip must be complete at a glance; hover may add, never reveal. |

---

## 12. Build plan — reuse vs new

### Reuse / refactor (priority order)

| Existing | Action |
|---|---|
| `RouteProgressBar.svelte` | **Extract rail internals into `CourseRail.svelte`** (props: `checkpoints`, `elements`, `shutoffs`, `density: 'panel' \| 'strip'`). `RouteProgressBar` and `dashboard/EventProgress.svelte` **already duplicate** the `wxBrackets` math, `elementColors` and `dotSize` verbatim — a third copy is unacceptable. This refactor pays for itself immediately. |
| `WxInterruptBanner.svelte` | **Generalize into `InterruptBanner.svelte`** (tier + queue + ack + focus-move + chime + vibrate). `WxInterruptBanner` becomes a thin wrapper. Ride EMERGENCY uses the same banner. Never build a second one. |
| `WxTierGlyph.svelte` + `wxAlertMeta.ts` | Copy the *pattern* into `RideTierGlyph.svelte` + `rideMeta.ts` — same table shape, same `--color-bg` punch-out. |
| `wxAlertTime.ts` | Use `countdown()`, `countdownTone()`, `clock()`, `clockWithDay()` **unchanged**. |
| `wxClock` / `wxMinute` (in `stores/wxAlerts.ts`) | **Promote to `$lib/stores/clock.ts`** as `secondClock`/`minuteClock`; re-export from `wxAlerts.ts`. One interval for the app. |
| `$lib/routeDistance.ts` | `buildRouteIndex`/`projectOnRoute`/`courseDistance` give lead and sweep mile markers from APRS positions with zero new geo code. This is why "miles remaining" [DOM] is cheap. |
| `stores/netcontrol.ts` | Subscribe to `netMetrics`, `attentionItems`, `orderedCheckpoints`, `progressElements`, `hasCheckpoints`. Add nothing. |
| `BottomSheet.svelte` | `peekContent` + `peekExtraH` is the entire mobile treatment, proven by `wx-peek-strip`. |
| `commandParser.ts` | Extend with `sag`, `shutoff`, `sweep`, `lead`, `close` verbs. Already handles `cp3 lead`. |
| `CommandPalette.svelte` | The composer for every multi-field strip action. |
| `annotationMeta.ts` | Checkpoint `statusMeta` already has Planned/Open/Active/Closed; add `Awaiting Sweep`. |
| `stationCategoryMeta.ts` | `sag` already exists (`#f97316`, short `SAG`). |

### Build new

**Frontend:** `RideStrip.svelte` (grid orchestrator) · `CourseRail.svelte` (extracted, shared 3 ways) · `RideTierGlyph.svelte` · `rideMeta.ts` · `stores/ride.ts` · `RidePendingList.svelte` · `ShiftBriefing.svelte`

**Backend:** `Shutoff`, `SweepReport`, `LeadReport`, `SagRequest`, `SagVehicle`, `RiderAccumulator`, `PendingReply`, `Shift` are WP2–WP5. `Phase` / `Net.RidePhase` is **WP5b**.

**TDD order per project law:** tests first for the `ride` package (shutoff arming/firing state machine, accumulator arithmetic across partial SAG pickups, pending-reply expiry, phase transition legality), then the manager, then wire through `internal/app`, then the frontend.

---

## Sources

**Domain ground truth** (established): edge-based rider accounting, sweep reporting, shutoff points, rolling course collapse, Marin ARS five-tier taxonomy, SAG request structure, supply batching, medical script, route-relative location grammar, two-person NCS with 2-hour rotation.

**External references:** Annunciator panel / dark cockpit / master warning · Airbus ECAM 4-level alert scheme · Turkish Airlines 1951 (cry-wolf via recurring self-clearing fault) · Alarm fatigue (WMATA 2009; FDA 566 deaths; Joint Commission 2013) · Google SRE (actionability test, symptom-based alerting, Four Golden Signals, alert bankruptcy) · Datadog Record/Notify/Page · Grafana NoData state and dashboard best practices · github/relative-time-element · NNGroup progress indicators (false precision) · RAG status (letters paired with color; amber drift) · Broom wagon (the moving unit *is* the cutoff boundary) · Comrades Marathon (hard cutoff as discrete event) · ICS 201 vs 214; ICS 219 T-cards · AHRQ PSNet I-PASS "synthesis by receiver" · Medical Priority Dispatch System.

**Explicitly unverified:** EEMUA 191 / ISA 18.2 numeric alarm-rate thresholds are paywalled and were not confirmed — the §4 sound budget is a conservative adaptation, not a quoted standard. Boeing EICAS specifics and FAA JO 7110.65 position-relief text were unreachable. Exact I-PASS effect sizes (NEJM 2014) are widely cited but were not re-verified.
