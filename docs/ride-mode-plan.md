# Bike Ride Mode — Build Plan

Specialty net-control mode for amateur radio volunteers supporting charity bicycle
rides (centuries, MS150-style events, gran fondos). Implemented as a **per-net
profile**, not a fork: one binary can run a SAR net and a ride net the same day.

Sourced from published operator handbooks (LOTOJA Amateur Radio Operators Bible,
Little Red Riding Hood Operations wiki, Marin ARS, SLOBC, AG6QR, Boston Marathon
COURSE Communications Plan, NET-104/110/PSV-105).

## Governing design facts

These are established from research and are **not** open to re-litigation during
implementation. They are the reason several obvious-looking designs are wrong.

1. **Rider accounting is edge-based, not roster-based.** Stations report a LEAD and
   a TRAILING rider per route. Individual riders enter the record only as
   exceptions (SAG'd, injured, DNF). There is **no rider roster and no bib-keyed
   table**. A bib number is a *label on an exception*, never a foreign key.
2. **Route-relative position is the primary coordinate system.** "Miles remaining"
   is the universal on-air position indicator. lat/lon is secondary.
3. **Course "clear" is not a boolean.** It is a rolling per-station collapse from
   the back forward; NCS holds the accumulator. The phrase itself is avoided on
   the air (it already means "I am done transmitting").
4. **Reconciliation closes by defining the population down.** Riders who pass a
   shutoff or decline SAG become *unsupported* and leave the accounted-for set.
   This is a status transition, not a deletion.
5. **Rest stops may not close until sweep passes them.** Real dependency.
6. **Medical traffic is notification-and-logistics only.** No clinical detail on
   the air. Ambulance on-scene and ambulance departed are separate logged events.
7. **Privacy:** bib numbers, not names. This is an ethical/served-agency rule, not
   a HIPAA statute on the operator. Some events withhold even the bib for severe
   injuries. Names ARE recorded for riders transported to a hospital or to the start.
8. **Agency-configurable.** We serve multiple ride organizers. Shutoff times, cut-offs,
   route distances, rest stop names, and priority vocabulary must be data, not code.
9. **Single net for v1.** Large events split into parallel Route/Rest Stop/Medical/
   Supply nets; we scope to one, but records carry a nullable `division` so multi-net
   lands later without a painful migration.

## Work packages

### WP1 — Net profile & configurable vocabularies  `backend`
- [ ] `Net.Profile` field (`general` | `bike-ride`), schema bump
- [ ] Profile registry: which panels mount, which annotation categories the palette
      offers, which check-in categories are suggested
- [ ] Profile-driven **traffic priority vocabulary**. Ride mode uses the five-tier
      Marin ARS ladder (EMERGENCY / PRIORITY / HIGH / MEDIUM / LOW) instead of the
      generic ARRL ladder. Tier definitions are config, with shipped defaults.
- [ ] Ride event config: route distances, cut-off policy, agency name/branding
- [ ] Backward compatibility: existing nets default to `general`, no behavior change

### WP2 — `internal/ride/` core: SAG requests  `backend`
- [ ] `SAGRequest`: pickup location (route-relative + optional coord), dropoff
      location, reason, priority tier, 1..N rider slots `{bib?, note, disposition}`
- [ ] Dropoff destinations: next rest stop / finish / start (their car) / hospital / other
- [ ] `SAGVehicle` capacity: seats + bike rack slots, committed vs available
- [ ] Status ladder incl. partial pickup and rider-self-resolved
- [ ] Assignment to a check-in with `category: sag`
- [ ] Events on the existing WebSocket hub
- [ ] REST API + persistence

### WP3 — Course closure, shutoffs & sweep  `backend`
- [ ] `ShutoffPoint`: coord + wall-clock time + reroute direction/destination +
      staffed-by assignment. Firing one is a logged, broadcastable event.
- [ ] Rider support status transitions (supported → unsupported via shutoff/declined SAG)
- [ ] Sweep tracking: last supported rider bib + estimated speed + position
- [ ] Rest-stop close gating on sweep passage
- [ ] Rolling course-clear accumulator per station, back-to-front
- [ ] Wire to existing `internal/checkpoint/` route progress

### WP4 — Supply & medical traffic  `backend`
- [ ] `SupplyRequest` with batching (prompt "what else?"), read-back confirmation
      step, and elapsed-time tracking ("ice truck said 20 min — 34 min ago")
- [ ] `MedicalNotification` fixed-field script: bib / sex / age / exact location /
      chief complaint / readback → ETA → on-scene → departed w/ destination + pt count
- [ ] On-scene and departed as separate timeline events
- [ ] Privacy guard: severe-injury bib withholding as a per-net policy flag

### WP5 — Reconciliation & reporting  `backend`
- [ ] Supported-rider accounting and the "all supported riders returned" close-out
- [ ] Post-ride checklist (field units checked out, net formally closed)
- [ ] SAG driver **shift summary** report: odometer start/end, transports, assists,
      tubes, tires, minor first aid, incidents attended
- [ ] ICS 211 (resource check-in/out) and ICS 214-RS / 214-SAG activity log exports,
      alongside the existing ICS-309
- [ ] Shift-relief briefing: pending activity / awaiting-reply carry-over

### WP5b — Ride phase model  `backend`  *(gap found by the dashboard study)*
Not in the original WP1-WP5 scope. The strip's content changes by ride phase, so
the phase must be real state, not a frontend guess.
- [ ] `Net.RidePhase`: pre-start / launched / mid-ride / closing / collapse / reconcile
- [ ] Legal transition table + tests
- [ ] **Operator-set, never silently auto-switched.** The system SUGGESTS a
      transition (trigger conditions per phase); NCS confirms. Silent state change
      is the classic ICS failure of an assignment nobody logged.
- [ ] Suggestion triggers: first lead passage -> launched; sweep passes stop 1 ->
      mid-ride; T-60 to first shutoff -> closing; NCS declares -> collapse;
      last stop clear -> reconcile
- [ ] Phase transitions are logged NetEvents

### WP6 — Ride status strip  `frontend`
**Full spec: `docs/ride-strip-spec.md`.** 128px persistent horizontal strip below
the map: course rail on top (the map's x-axis), eight zone tiles below. Separate
component from `SituationBoard.svelte`, which becomes its detail view.
- [ ] Extract `CourseRail.svelte` from `RouteProgressBar.svelte` (see debt below)
- [ ] Generalize `WxInterruptBanner.svelte` -> `InterruptBanner.svelte`; ride
      EMERGENCY reuses it. Never build a second full-width interrupt banner.
- [ ] Promote `wxClock`/`wxMinute` to `$lib/stores/clock.ts`; one interval app-wide
- [ ] `RideStrip.svelte`, `RideTierGlyph.svelte`, `rideMeta.ts`, `stores/ride.ts`
- [ ] `--ride-strip-h` published to `:root` at mount (the `--sheet-peek` recipe);
      `.map-layer` inset off it; `map.invalidateSize()` on every height change
- [ ] Mobile: 44px peek line in `BottomSheet`, three facts only

### WP7 — Ride mode UI  `frontend`
- [ ] Panel gating by profile (hide APRS-heavy UI)
- [ ] SAG request composer and board
- [ ] Rest stop status board
- [ ] Shutoff countdown + fire control
- [ ] Supply and medical composers matching the scripted field order
- [ ] Mobile/tablet treatment

### WP8 — UI polish pass  `frontend`
- [ ] Audit against CLAUDE.md frontend law: hierarchy, density, consistency,
      responsiveness, accessibility, feedback
- [ ] Design tokens only — no ad hoc color/spacing
- [ ] No status encoded in color alone
- [ ] Keyboard navigation and contrast verification

### WP9 — Pre-existing debt surfaced by the dashboard study
- [ ] **Contrast defect.** `--color-error` (#e74c3c) is 4.16:1 and `--color-info`
      (#3b82f6) is 4.32:1 against `--color-surface` — both FAIL WCAG AA for
      normal-size text. Confined to glyphs/borders in the strip spec, but this
      likely affects existing components. Audit and fix repo-wide.
- [ ] **Triple duplication.** `RouteProgressBar.svelte` and
      `dashboard/EventProgress.svelte` already carry verbatim copies of the
      `wxBrackets` math, `elementColors` and `dotSize`. The strip would be a
      third. Fixed by the `CourseRail.svelte` extraction in WP6.

### WP10 — Docs
- [ ] Wiki: roadmap check-offs + current status
- [ ] Operator-facing ride mode guide
- [ ] Wiki submodule push order: `cd wiki && git push origin master` FIRST, then main repo

## Test fixtures
`Day_1_48M_Jack_and_Back.gpx` and `docs/GR_2025_100_miler.kml` — real courses.
