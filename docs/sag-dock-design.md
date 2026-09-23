# SAG dock — actions & zoom design note

Acceptance spec: `docs/sag-dock-scenarios.md` (S1–S10). Original dock spec:
`docs/sag-map-spec.md` §1/§2/§8/§11. Visual recipes:
`docs/ride-dashboard-design-pass.md` §3. This note records, per scenario, the
gap in the dock as it stood and the design answer that shipped.

## The interaction model in one paragraph

Every **active leg** (dispatched, en route, on scene, loaded) renders as a
*leg row* inside its request card — and inside its vehicle's row in the new
Vehicles view. A leg row is two lines: a status line (`KG4YFA-4 · EN ROUTE · 4m`)
and ONE primary button that names the vehicle and the state it moves to
(`KG4YFA-4 → On scene`), with a `⋯` More button at the far right. The primary
is always the *next rung* of the ladder: en route, on scene, then `Loaded…` and
`Delivered…` which open a compact inline confirm with the common answer
prefilled. Skips (en route → loaded), rider exceptions (self-resolved, declined,
not found, handed off) and Release live behind `⋯`, and each asks for a second
confirm. Clicking a card fits the map to that job; a header button fits all
SAG. The rules that make it safe: the button's label says exactly what it will
write; after a commit the button becomes `✓ <vehicle> on scene` and is inert
for 1.5 s (a double-tap cannot advance twice while the label re-renders under
the finger); an inline confirm never puts its Confirm button where the button
that opened it was, and ignores taps for its first 400 ms.

## Per scenario

**S1 — "SAG 2, on scene"** · *Gap:* once a driver was assigned the card showed
no leg and no action; In-motion was collapsed. · *Answer:* leg rows with a
primary `<vehicle> → On scene`; In-motion is now open by default (it is where
S1–S3 happen; it stays below Waiting, so blocked-on-me still leads). The
Vehicles view (S6) finds the unit by the name the driver said. Feedback: the
button turns into `✓ … on scene`, the status line updates, and the polite live
region announces it.

**S2 — "has both riders and both bikes"** · *Gap:* load only on the board. ·
*Answer:* `→ Loaded…` opens an inline confirm: a one-line summary
(`2 riders · 2 bikes on the rack`) and `Confirm load`. `Change…` expands one
row per rider: Aboard / Not aboard (stays waiting) / Declined, and the bike
disposition (same `BIKE_ORDER` select as the board). Declined is a load
followed by a resolve — the server detaches an unloaded rider back to waiting,
so the resolve is then legal. Same API calls as SagBoard (`loadSagSlots`,
`resolveSagSlot`).

**S3 — "dropped them at Flat Creek"** · *Gap:* deliver only on the board. ·
*Answer:* `→ Delivered…` opens `Deliver 2 riders at <dropoff>` + `Confirm
delivery`, naming the planned dropoff. `Change…` offers per-rider selection
and "somewhere else" (the board's destination kinds). A completed request drops
out of the dock's groups on the next board update (it already filtered
complete/cancelled).

**S4 — "fixed the flat and rode on"** · *Gap:* resolve only on the board. ·
*Answer:* `⋯` → *Rider outcome* shows one row per rider on that leg:
`Rode on` / `Declined` / `Not found` (a loaded rider: `To medical`) → a
confirm that spells the outcome and its consequence (`Bib 512: Fixed own flat,
rode on? This frees KG4YFA-4.`). The `⋯` button is at the opposite end of the
row from the primary and opens *below* it; nothing destructive is ever one tap.
Release is in the same menu, through the board's `ReasonDialog`.

**S5 — "Where is SAG 2?" / "What's at mile 30?"** · *Gap:* card click selected
but did not move the map. · *Answer:* card click (or `z`) fits the job's
extent — pickup, dropoff and every active leg's vehicle with a position —
through `+page.svelte`'s `handleFlyToBounds` + `sagFitPadding`, so the fit is
clear of the dock/rail/sheet. The dock never touches Leaflet; it calls an
`onFitPoints` prop. A vehicle row zooms to its last fix; a fix older than the
stale threshold says `stale 24m` in the row and in its screen-reader name,
never a bare position. A single point is padded to a ~1 km box so the map does
not slam to max zoom. An unplaceable pickup still moves the map nowhere
(spec §7). The header's `⤢` fits every open pickup and every SAG vehicle on
the map. On a phone a fit drops a full-height sheet to half so the result is
visible.

**S6 — "Who's free?"** · *Gap:* no vehicle view. · *Answer:* a two-tab switch
under the header — `Requests (n)` / `Vehicles (n)`, remembered per browser.
Vehicles are listed in **stable natural order by name** (a list you search by
name must not reshuffle as states change): name, `3/3 seats · 2/2 racks free`
or FULL, position age (`4m`, `stale 24m`, `no fix`), `Free` when idle, then one
leg row per job — a van with a second pickup shows both, each with its own
primary. A unit with no fix offers `Place on map` there too.

**S7 — mis-tap safety** · The primary names vehicle and target; skips are only
in `⋯` and need a second confirm; the post-commit lock and the moved Confirm
button defeat double-taps; `Loaded`/`Delivered` always confirm. Every commit
shows the ✓ state; a server refusal is shown inline on the leg
(`⊘ Could not mark SAG 3 en route — cannot transition leg…`), scrolled into
view, as well as in a toast, and stays until the next action on that leg.

**S8 — keyboard** · Still one tab stop with roving focus; leg rows and vehicle
rows are now roving items too. `Enter` on a leg row runs its primary (the row's
accessible name says what that is); `o` opens its More actions; `z` zooms to
the focused item; `Shift+Z` zooms to all SAG; `v` switches Requests/Vehicles;
`f` opens a find-unit field — type `sga`, Enter lands on that vehicle and
zooms to it. `Esc` closes an open confirm/menu first, then collapses the rail.
Existing `d`/`p`/`m`/`i` unchanged. Opening a confirm moves focus to its
commit button, and its controls join the tab order only while it is open.
Handled keys stop propagating; none collides with the strip's global keys, and
`a` is deliberately *not* the advance key because it acknowledges an emergency
everywhere. Every commit is announced through the dock's single polite region.

**S9 — every surface** · Same component on desktop (260px), tablet rail →
overlay, and the phone sheet's `sag` mode. Leg primaries, `⋯`, tabs and
confirm buttons are 44px under the existing `max-width: 1199px` block (36px
desktop-compact otherwise, per the design pass's button ladder); nothing is
hover-only (`title` is supplementary). Key hints (`<kbd>`) are hidden below
1200px (design pass §5.7).

**S10 — busy ride** · Group order unchanged: `⚑ Not on the map`, Waiting
(by urgency), In motion, No position. A waiting card is unchanged in height;
an in-motion card is card row + a two-line leg row (~100px, about a waiting
card), not double. The confirm/menu panels expand only for the one leg being
worked. The tablet rail keeps its counts-only design.

## Things deliberately not done

- No backend change: every verb already existed.
- No hold-to-confirm on the primary: the radio call *is* the confirmation and
  the operator does several per minute; the lock + explicit label is the
  cheaper guard.
- Requests are still labelled `SAG <n>` (the board and on-air convention). A
  vehicle whose tactical call is also `SAG <n>` is ambiguous with a request
  number; vehicle names therefore always render in the SAG-unit colour with the
  unit wording in their accessible names. Renaming requests is out of scope.
