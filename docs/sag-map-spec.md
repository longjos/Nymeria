# SAG on the Map — Design Spec

**Components:** `SagMapLayer` (an `$effect` family inside `Map.svelte`) · `SagDock.svelte` · `SagCandidatePanel.svelte` · `sagGeo.ts` · `sagDispatch.ts`
**Mode:** bike ride mode only (`$rideMode`, i.e. `Net.Profile === 'bike-ride'`)
**Placement:** on the map, plus a left-edge dock (desktop/tablet) / bottom sheet (phone)

Tags: **[DOM]** = established domain research · **[REF]** = external reference · **[DJ]** = design judgment · **[Q]** = open question for the operator, deliberately not invented.

> **SCOPE NOTE:** everything this spec needs on the backend already exists and is
> shipped — `SAGRequest`, `SAGSlot`, `SAGLeg`, `SAGVehicleStatus`, `SAGBoard`,
> `SAGLocation`, `NetCheckIn`, the `sag_request_*` / `sag_vehicle_updated` WS
> events, `POST .../legs` dispatch with its 409 over-capacity body. **This is a
> frontend-only feature plus one pure function in `routeDistance.ts`.** If an
> implementer finds themselves adding a Go type, they have left the spec.

> **SIBLING:** `docs/ride-strip-spec.md` owns the 128px strip below the map and
> the `--ride-*` token family. This spec owns the map surface above it and adds
> exactly four tokens. Where the two disagree about a shared rule (staleness
> words, tier colour discipline, keyboard inertness), the strip spec wins.

---

## 0. The decision the map is serving

A rider calls in on the radio: *"Rider down, mile 34, north side, one rider, bike
is rideable."* Net control has somewhere between ten and sixty seconds — on a
shared simplex channel, with the caller still holding the mic — to answer one
question: **which vehicle do I send, and can it carry them?** Today that answer
is assembled in the operator's head from three disconnected places: `SagBoard`
tells them a request exists at mile 34, a seat/rack strip tells them SAG 3 has
two free seats, and the map — the only surface that knows SAG 3 is eleven miles
back and on the wrong side of a closed shutoff — is not consulted at all,
because nothing on it says "SAG". Everything in this spec is designed backwards
from collapsing those three lookups into one glance and one click.

Three corollaries, which are the whole spec in miniature:

1. **The map must answer "how far", not "how near".** Crow-flies proximity on a
   road course is a lie with a ridge in it. Ranking is by route distance
   (`courseDistance`) wherever a course exists, and where it does not the map
   says so in words rather than quietly substituting a straight line. [DJ]
2. **A pickup that cannot be drawn is still a pickup.** `SAGLocation.lat/lon`
   are optional and usually absent — on-air position is route-relative [DOM
   fact #2]. An overlay that renders only the placeable requests would show a
   calm map during the exact minute three riders are waiting at unplaceable
   locations. Unplaceable is a **first-class rendered state** (§3), not a filter.
3. **Load is part of the answer, not a detail behind a click.** "Send SAG 3" is
   wrong if SAG 3 has one seat and there are two riders. Capacity is on the
   vehicle marker, at rest, with no interaction (§4).

---

## 1. Wireframes

### Desktop (>=1200w) — map, dock, strip

```
┌─ SAG DOCK (260px) ──────┐┌───────────────── MAP ──────────────────────────────┐
│ SAG · 3 waiting         ││                                                     │
│ ─────────────────────── ││            ╭─────────╮                              │
│ ⚑ NOT ON THE MAP    (1) ││            │ SAG 3 ▮▮▯▯│ ← vehicle chit (seats top,  │
│ ┌─────────────────────┐ ││            │       ▭▭▭│    racks bottom)            │
│ │ PR SAG 7 · 1 rider  │ ││            ╰─────────╯                              │
│ │ "just past the barn"│ ││       ●━━━━━━━━━━━●━━━━━━━━━━━━━━━●━━━━━━━━━●       │
│ │ 14m · [Place on map]│ ││      RS1          ▽            RS2        FIN      │
│ └─────────────────────┘ ││                   PR                                │
│                         ││                   2   ← pickup pin: tier PR,        │
│ ⬒ WAITING           (2) ││                        2 riders, dashed ring =      │
│ ┌─────────────────────┐ ││                        NEEDS VEHICLE                │
│ │▌PR SAG 5 · mi 34.0  │ ││           ╭─────────╮                               │
│ │  2 riders · 6m      │ ││           │SAG 1 ▮▮▮▮│ ⊘   ← seats full: hard stop  │
│ └─────────────────────┘ ││           │      ▭▭▭|▨│     rack overflow nub       │
│ ┌─────────────────────┐ ││           ╰────┈┈✛───╯     hand-placed (dotted tail)│
│ │  HI SAG 6 · mi 51.2 │ ││                                                     │
│ └─────────────────────┘ ││         ╭╌╌╌╌╌╌╌╮ 24m stale — drift halo            │
│                         ││        ╭┤ SAG 2 ├╮                                  │
│ 🚐 NO POSITION      (1) ││        ╰╌╌╌╌╌╌╌╌╌╯                                  │
│ │ SAG 4 · voice, never │ ││                                                    │
│ ─────────────────────── ││                                                     │
│ [ SAG focus ]  [ ⌄ ]    ││                                          ⚑ dropoff  │
└─────────────────────────┘└─────────────────────────────────────────────────────┘
╔════════════════════ RIDE STRIP (128px, ride-strip-spec) ═══════════════════════╗
```

### Dispatch focus — the centrepiece

Triggered by clicking a pickup pin, pressing `d` on a focused request, or
clicking a `SagBoard` row's **Find a driver** button.

```
┌─ CANDIDATES for SAG 5 ──┐┌───────────────── MAP (everything else at 30%) ─────┐
│ mi 34.0 · 2 riders      ││                                                     │
│ ⬒ needs a vehicle · 6m  ││        ╭─────────╮                                  │
│ ─────────────────────── ││       ②│ SAG 3 ▮▮▯▯│                                │
│ ① SAG 1    4.2 mi ahead ││        ╰────┬────╯                                  │
│    ~10 min              ││             ╎ 7.8 mi ahead · ~19 min                │
│    ▮▮▯▯ 2 seats free    ││             ╎                                       │
│    ▭▭▭▭ 2 racks free    ││    ●━━━━━━━━━┿━━━━━━━━━●━━━━━━━━━━━━━━━━●           │
│         [   Send   ]    ││              ▽ SAG 5                                │
│ ─────────────────────── ││             ╎PR                                     │
│ ② SAG 3    7.8 mi ahead ││             ╎2                                      │
│    ~19 min              ││  ╭─────────╮╎                                       │
│    ▮▮▯▯ · ▭▭▭▭          ││ ①│ SAG 1 ▮▮▯▯├╯ 4.2 mi ahead · ~10 min              │
│         [   Send   ]    ││  ╰─────────╯                                        │
│ ─────────────────────── ││                                                     │
│ ⊘ SAG 4    1.1 mi ahead ││    (greyed, no leader line — cannot seat 2)         │
│    seats 1 of 2 needed  ││                                                     │
│    [ Send 1 rider ]     ││                                                     │
│ ─────────────────────── ││                                                     │
│ ? SAG 2    position 24m ││                                                     │
│    old — stale          ││                                                     │
│ ─────────────────────── ││                                                     │
│ Esc to exit       ↑↓ ⏎  ││                                                     │
└─────────────────────────┘└─────────────────────────────────────────────────────┘
```

### Phone (<769px) — dispatch focus in the bottom sheet

```
┌─────────────────────────────────────┐
│                                     │
│         MAP, auto-fit to            │   fit padding accounts for the
│    pickup + top-3 candidates        │   sheet height, exactly as
│                                     │   --sheet-peek already does
│            ▽ SAG 5                  │
├─────────────────────────────────────┤ <- BottomSheet, 'half'
│ ━━━━                                │
│ SAG 5 · mi 34.0 · 2 riders · 6m     │
│ ┌─────────────────────────────────┐ │
│ │① SAG 1  4.2 mi ahead  ~10 min   │ │  each row >= 56px
│ │  ▮▮▯▯ 2 seats  ▭▭▭▭ 2 racks     │ │
│ │                    [  Send  ]   │ │
│ └─────────────────────────────────┘ │
│ ┌─────────────────────────────────┐ │
│ │② SAG 3  7.8 mi ahead  ~19 min   │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
```

No dock on phone. The dock's *content* is reachable as a `sag` mode of the
bottom sheet; the map keeps its markers.

---

## 2. Placement — how a `SAGLocation` becomes a map point

This is the highest-risk part of the feature and is therefore built and tested
first (§12). A wrong pin is worse than no pin: it will be believed.

### The resolver

`resolveSagPoint(loc: SAGLocation, ctx): SagPlacement` in a new `$lib/sagGeo.ts`,
where `ctx = { routeIndex: RouteIndex | null, routeTotalMeters, annotationsById, stops: Stop[] }`.

```ts
type SagPlacement =
  | { placed: true;  lat: number; lon: number; via: PlacementVia; chainageMeters: number | null; label: string }
  | { placed: false; reason: UnplaceableReason; label: string };

type PlacementVia = 'coordinate' | 'annotation' | 'mileage' | 'named-stop';
type UnplaceableReason =
  | 'no-course'          // mileage given, but no route annotation is loaded
  | 'no-location'        // free text / bare kind only — "just past the red barn"
  | 'route-unknown'      // loc.route names a route we have no geometry for
  | 'off-route';         // mileage is outside [0, routeTotal]
```

Resolution order — **first hit wins, and the winner is recorded in `via`,
because the map renders confidence differently per source (§3.4)**:

| # | Condition | Result | `via` |
|---|---|---|---|
| 1 | `lat != null && lon != null` | use them verbatim | `coordinate` |
| 2 | `annotationId` resolves to an annotation with `Point` geometry | that point | `annotation` |
| 3 | `mileMarker != null` **and** a `RouteIndex` exists | `pointAtChainage(idx, mileMarker * 1609.344)` | `mileage` |
| 4 | `milesRemaining != null` **and** a `RouteIndex` exists | `pointAtChainage(idx, idx.totalMeters - milesRemaining * 1609.344)` | `mileage` |
| 5 | `kind ∈ {next_reststop, finish, start}` and the corresponding stop is resolvable | that stop's point | `named-stop` |
| 6 | otherwise | unplaceable, with the reason from the table above | — |

**Why mileage → point is safe when point → mileage is not.** `projectOnRoute`
returns *multiple* candidates on an out-and-back, because two places 60 miles
apart along the course can be 25 m apart on the ground (`routeDistance.ts`
documents this on the user's own 100-miler). The inverse lookup has no such
problem: a chainage is a single scalar and identifies exactly one vertex pair.
This asymmetry is why **mileage-derived pickups are the most trustworthy pins on
the map and GPS-derived vehicle positions are the least**, which is the opposite
of what a mapping engineer's intuition says. [DJ]

### New pure function — the only non-component code this feature adds

```ts
/** The point `meters` along the course from its start. The exact inverse of
 *  Candidate.chainageMeters. Clamps to the ends on an open course; wraps on a
 *  closed loop. Linear interpolation inside the containing segment. */
export function pointAtChainage(idx: RouteIndex, meters: number): { lat: number; lon: number } | null;
```
Binary search on `idx.cum`, lerp between `lats[i]/lons[i]` and `lats[i+1]/lons[i+1]`.
Refuses (`null`) when `meters` is outside `[0, totalMeters]` on an open course —
which is how `off-route` is detected. Lives in `routeDistance.ts` beside
`courseDistance`, table-driven tests against both real fixtures
(`Day_1_48M_Jack_and_Back.gpx`, `docs/GR_2025_100_miler.kml`), including the
round-trip property `projectOnRoute(pointAtChainage(idx, m))[0].chainageMeters ≈ m`.

### Which route index

`SAGLocation.route` is a route *name* (`"100"`, `"48"`), and a ride can carry
several. The resolver takes the route annotation whose `label` or `shortName`
matches `loc.route`; with no `route` field it takes the single route annotation
if there is exactly one, and returns `route-unknown` if there are several and
nothing says which. **Silently picking the first of three routes would
mis-place a pin by tens of miles.** [DJ]

**[Q1]** Do the user's events actually run multiple route annotations in one
net, or one net per route? If one-per-net, rule 6's `route-unknown` branch is
dead code and the resolver simplifies.

### The `milesRemaining` / configured-distance mismatch

`milesRemaining` is converted against `idx.totalMeters` — the *measured* length
of the imported GPX — not against `rideConfig.routes[].distanceMiles`, the
*advertised* length. These routinely differ by 1–3%. If they differ by more than
**2%**, the dock shows a single one-time notice: `Course measures 98.4 mi;
config says 100 mi — "miles remaining" pins use the measured length.` No
auto-correction, because we cannot know which number the field stations are
reading off their cue sheets.

**[Q2]** When a rest stop calls in "mile 34", is that the *advertised* mile
marker painted on the road, or their read of a GPS? If it is the painted
marker, we should scale mileage by `configDistance / measuredDistance` and the
rule above inverts. This materially changes pin accuracy and I will not guess it.

### The unplaceable case — designed, not tolerated

Unplaceable requests render in the dock's **`⚑ NOT ON THE MAP`** group, which is
**always the top group** and is the only group that renders when it is empty-but-
nonzero... it renders only when count > 0, but it renders *above* everything,
including higher-tier placeable requests. Rationale: a placeable EMERGENCY is
already screaming on the map; an unplaceable PRIORITY is invisible everywhere
else and this dock group is its only home. [DJ, dark-cockpit inversion]

Each card carries:

```
┌───────────────────────────────┐
│ PR  SAG 7 · 1 rider           │   tier glyph + code (RideTierGlyph)
│ "just past the barn on 240"   │   loc.description verbatim, quoted
│ Not on the map — no mile      │   the REASON, in words
│ marker and no coordinate      │
│ 14m                           │
│ [ Place on map ] [ Add mile ] │
└───────────────────────────────┘
```

**`Place on map`** is the important one. It enters the existing pick mode:
`placingSagLocation = { requestId, which: 'pickup' | 'dropoff' }`, a new member
of `Map.svelte`'s `PickSource`/`anyPlaceMode` family, resolved through the same
`commitPick` choke point `placingOperator` and `placingAnnotation` already use.
The click writes `{lat, lon}` back through `api.updateSagRequest`. **The map is
not just a display for this feature; it is the tool that repairs the missing
coordinate.** Zero new interaction machinery — `layerClick`/`commitPick` already
does all of it.

`Add mile` opens the existing `SagLocationField` in the request editor with the
mile-marker input focused, which is faster than a map click when the caller is
still on the air saying a number.

Per-reason copy:

| `reason` | Card says | Actions offered |
|---|---|---|
| `no-location` | "Not on the map — no mile marker and no coordinate" | Place on map · Add mile |
| `no-course` | "No course loaded — mile 34 can't be drawn" | Import course (→ `navigateZone('course-import')`) · Place on map |
| `route-unknown` | "Route \"{route}\" isn't loaded" | Import course · Place on map |
| `off-route` | "mi {n} is past the end of the {len} mi course" | Add mile · Place on map |

---

## 3. Marker language

Three things, three silhouettes. The test each must pass: **printed in
greyscale, at 100% zoom, viewed at arm's length on a 6" phone in sunlight, can
you name which of the three it is without a legend?** [REF: RAG status — letters
paired with colour; colour is never the only channel]

### 3.1 Pickup — a downward pin

```
      ╭───────╮        32 × 38 px (desktop) / 40 × 46 (touch)
      │  PR   │        line 1: tier CODE, two letters, --color-text
      │   2   │        line 2: rider count needing a vehicle (numeral)
      ╰──╮ ╭──╯        anchor: the apex, at the point
         ▽             fill: tier soft token; border 2px tier token
```

- **Geometry:** a rounded-square body with a downward apex. Reads as "the
  problem is *here, at this spot on the ground*".
- **Colour:** `border: 2px solid var(--color-ride-{tier})`, `background:
  var(--color-ride-{tier}-soft)` over a `--color-surface` base so it is opaque
  against tiles. Tier tokens come from `tierStyle(tier)` — **never a hardcoded
  tier name; the ladder is agency-configurable** [DOM fact #8], so the component
  reads `tierById($rideLadder, r.priority)` exactly as `SagBoard` does.
- **Text:** the two-letter code renders in `--color-text`, never in the tier
  colour, because `--color-ride-emergency`/`-medium` alias `--color-error`/
  `--color-info`, which fail AA for normal text (`app.css` says so in a
  comment). Same conclusion the strip spec reached. [DJ, computed]
- **`needsVehicle`** — the single most important bit on the map — is encoded
  three ways at once: a **dashed 2px outer ring** around the pin, the **rider
  numeral** (which is `unassignedRiders(r)`, and is *absent* when zero), and a
  slow **1.6s two-cycle pulse on arrival only**, killed by the global
  `prefers-reduced-motion` rule. Nothing pulses continuously. [strip spec §4]
- **Assigned but not yet loaded:** ring becomes solid, numeral disappears, and a
  leg line appears (§5). The pin does not change colour — the tier did not change.
- **Stroke halo:** every pin gets the existing `--wx-casing`
  (`rgba(255,255,255,0.55)`) 1px outer casing, the precedent already established
  for map strokes that must survive a bright tile underneath.

### 3.2 Dropoff — a pennant

```
      ⚑───╮       20 × 24 px; anchor: the pole's foot
      │   │       fill: --color-sag-dropoff (= --color-text-muted)
      │▨▨▨│       hospital dropoff: the pennant is hatched + carries ✚
      ┃            pole: 1px, 10px tall
      ┃
```

- Deliberately **smaller, quieter and never tier-coloured**. The destination is
  not urgent; the pickup is. Making dropoffs compete with pickups for attention
  is the single easiest way to ruin this overlay.
- Only one dropoff variant is loud: `kind === 'hospital'` gets a `✚` and the
  hatched fill, because a hospital dropoff means the vehicle leaves the course
  and is out of the rotation for an hour — an operational fact worth seeing.
- **Dropoffs are drawn only for the focused request** (§5). All of them at once
  is a field of grey pennants on top of the rest stops they usually *are*.

### 3.3 Vehicle — a chit, not a pin

```
  ╭──────────────────╮      56 × 28 px (desktop) / 64 × 32 (touch)
  │ SAG 3    ▮▮▯▯   │      hit target padded to 44 × 44 regardless
  │          ▭▭▭▭   │      anchor: CENTRE (it is an area, not a point)
  ╰──────────────────╯
```

A **horizontal rounded rectangle** — a card, not a pin. This is the whole reason
it is instantly distinguishable: pins point down at a spot, the chit sits *on*
one. Silhouette alone separates it from both other markers in greyscale.

- **Label:** `v.tacticalCall || v.callsign`, truncated to 7 chars with an
  ellipsis; full name in the tooltip and the `aria-label`.
- **Border/text:** `--color-sag-unit` (`#f97316`, the colour
  `stationCategoryMeta.ts` already assigns to the `sag` category — **5.67:1 on
  `--color-surface`, passes AA as text**; computed, see §9).
- **The chit replaces the station's APRS marker.** When a station is a
  checked-in SAG vehicle, `Map.svelte`'s `updateMarkers()` skips it and the SAG
  layer draws the chit. The chit already carries the callsign; drawing both is
  the same double-labelling the existing operator-halo code already declines to
  do. This is the single largest clutter reduction available and it is free.

### 3.4 Load — seats and racks, at rest, with no click

**The ask: "what their load looks like right there on the map."** Two stacked
segmented bars on the chit's right. **Top row is always seats. Bottom row is
always racks.** Position is the channel; there are no letter prefixes to read.

```
  seats   ▮▮▯▯      4 seats, 2 committed          filled = committed
  racks   ▭▭▭▭      4 racks, 0 committed          open   = available
```

Seats and racks are **not symmetric**, and the marker says so — this is the
design's load-bearing idea:

| State | Seats (a seatbelt) | Racks (a truck bed) |
|---|---|---|
| available | `▮▮▯▯` | `▭▭▭▭` |
| exactly full | `▮▮▮▮` + **frame thickens to 2px and closes** | `▬▬▬▬` frame stays 1px |
| over | **impossible** — the server refuses; `⊘` badge on the chit when `availableSeats <= 0` | `▬▬▬▬│▨` — a **hatched nub drawn outside the frame**, physically depicting the bike in the bed |

A seat limit is a wall you can see; a rack limit is a line you can see something
spilling over. That is exactly the distinction the existing capacity dialogs
already make in words ("a seat refusal STATES A LIMIT; a rack overflow ASKS A
QUESTION" — `SagBoard.svelte`), rendered as geometry. Greyscale-safe: filled vs
open segments, plus a nub that is outside the frame, plus a `⊘` glyph.

**Segment budget.** At most 6 segments per row. Above 6, the row collapses to a
single proportional bar plus a numeral: `▬▬▬▬▬▬ 9/12`. Above 6 the pip count is
unreadable at 8px anyway and the numeral is more honest.

**`aria-label` and tooltip** always carry words, never geometry:
`"SAG 3, 2 of 4 seats committed, 0 of 4 racks committed, position 4 minutes old"`.

### 3.5 Staleness and position source

Staleness is about the **vehicle's position**, not the vehicle. Thresholds and
vocabulary reuse `rideMeta.ts`'s `ageState`/`ageText` and the strip spec's four
named states verbatim — `fresh` / `aging` / `stale` / `never`. No new thresholds.

| Position state | Chit rendering |
|---|---|
| `fresh` (<10m) | solid 2px border, no age tag |
| `aging` (10–20m) | solid 2px border, amber age tag `14m` in the chit's corner |
| `stale` (>20m, `STALE_THRESHOLD_MS`) | **dashed** border, muted label, age tag `24m`, and a **drift halo** — a `--color-sag-drift` filled circle centred on the last fix, radius = `minutesOld × 35 mph`, capped at 8 mi. Plus the literal word `stale` in the tooltip and the dock. |
| `never` (no `lat`/`lon` at all) | **not drawn on the map at all.** Goes to the dock's `🚐 NO POSITION` group, which says `voice check-in, position never set` and offers **`Place on map`** — the same pick mode as §2, writing through the existing operator-placement path (`onOperatorPlaced`). |

**The drift halo is the honest answer to constraint 2.** A 40-minute-old APRS
fix is not where the driver is, and drawing a crisp chit there says it is. The
halo says "somewhere in here" and, critically, **it is visually enormous** —
which is the point: a stale unit *should* look untrustworthy next to a fresh
one. The 35 mph figure is a display convention, not a claim.

**[Q3]** Is 35 mph a sane upper bound for a SAG van on these courses (rural
two-lane, closed or shared with riders)? If they run on highways between rest
stops, the halo should use 55 and will be much bigger.

Source is encoded by the **tail**, not by colour:

| `NetCheckIn.source` | Chit |
|---|---|
| `aprs` | no tail — the chit floats on its own fix |
| `voice` (hand-placed) | a 1px **dotted tail** down to a small `✛` ground mark, and the tooltip reads `position set by hand at 10:42` |

A hand-placed position still ages and still gets a drift halo past 20 minutes —
the van drove away regardless of who typed the coordinate.

---

## 4. Suppression, panes and z-order

The map already carries stations, tracks, DR cones, annotations, the route line,
operator halos, mission flags, assignment lines, weather stations, NWS alert
polygons and DF cones. **Adding a tenth competing layer is the most likely way
this feature fails.** [DJ]

### Panes

Two new Leaflet panes, created in `Map.svelte`'s `onMount` beside `wxAlertPane`,
with z-indices read from CSS tokens exactly as `readWxTokens()` reads `--z-wx-pane`:

```
tilePane            200
wxAlertPane         350   (NWS polygons — existing)
overlayPane         400   (annotations, the route line)
sagLinkPane         420   <- NEW: leg lines, pickup→dropoff lines
shadowPane          500
markerPane          600   (stations, halos, mission flags)
sagMarkerPane       620   <- NEW: pickup pins, dropoff pennants, vehicle chits
tooltipPane         650
popupPane           700
```

Link lines sit **above** the route line (so a leg is never buried under the
course) and **below** every marker (so a line never crosses over a pin's
numerals). Markers sit above station markers because during a SAG decision the
SAG markers *are* the subject, and below tooltips so labels still win.

### What is suppressed, and when

| Layer | SAG overlay on | Dispatch focus |
|---|---|---|
| Route line | **kept** — it is the coordinate system [DOM fact #2] | kept, full opacity |
| Checkpoints / rest stops | **kept** — they are the dropoff vocabulary | kept, full opacity |
| Vehicle's own APRS station marker | **replaced by the chit** (§3.3) | replaced |
| Generic `netAssignmentLines` | **suppressed in ride mode** — a dashed blue line between two points is the same grammar as a leg line and would be unreadable next to it. Missions are near-unused in ride mode. | suppressed |
| Net operator halos | kept for non-`sag` categories; a `sag` check-in's halo is suppressed (the chit is its marker) | dimmed |
| Other stations, tracks, DR cones, callsign labels | kept | **dimmed to 30%** |
| Annotations (non-course, non-stop) | kept | **dimmed to 30%** |
| Weather stations, DF cones | kept | **dimmed to 30%** |
| NWS alert polygons | kept (pane 350, under everything; heat is a real SAG driver) | kept — an operator sending a van into a severe-storm polygon needs to see it |

Dimming is one CSS class toggled on the map container
(`.map--sag-focus .leaflet-marker-pane > :not(.sag-marker) { opacity: .3 }`), not
layer removal. Reversible, no relayout, no `$effect` churn, survives `Esc`.

### The `SAG focus` preset

The dock's footer and `MapPalette` both carry one button: **`SAG focus`**. It
sets `showTracks: false, showDRCones: false, showCallsigns: false,
showWeatherOverlay: false, showDFOverlay: false, stationAgeFilter: '1h'` in one
click, and a second press restores the previous values (held in a
`preSagFocusSettings` snapshot). `MapPalette`'s existing `hasNonDefault`
indicator dot already communicates that the map is filtered, so nothing new is
needed to prevent the classic "why is my map empty" failure.

---

## 5. Lines — the two relationships

All-lines-always is spaghetti; this is the rule set that prevents it.

### 5.1 Pickup → dropoff

- **Drawn only for the focused request.** Never for all requests.
- Style: 2px, `--color-sag-dropoff`, `dashArray: var(--sag-dash-pickup-dropoff)`
  (`2 7` — the sparsest dash in the app, deliberately quieter than every wx
  dash), no casing, `interactive: false`.
- A single `›` chevron decorator at the midpoint gives direction.
- **Straight line, not a route trace.** We have no routing engine and drawing
  the course between them would imply the van follows the course, which it often
  does not. A straight tie-line reads as "these two things belong together",
  which is all it needs to mean. [DJ]

### 5.2 Vehicle → assignment (leg)

Drawn from the **vehicle chit to the point the vehicle is going next**, which
changes as the leg advances. This is the whole trick: the line always answers
"where is this van headed", never "what is this van historically attached to".

| `leg.status` | Line target | Style |
|---|---|---|
| `dispatched` | pickup | 2px tier colour, `dashArray: var(--sag-dash-leg-dispatched)` (`6 5`) — dashed = *told, not yet rolling* |
| `enroute` | pickup | 2px tier colour, **solid** |
| `onscene` | pickup | 2px tier colour, solid, with a small ring at the pickup |
| `loaded` | **dropoff** | 2px tier colour, solid — the line swings to the destination the moment the riders are aboard |
| `delivered` / `released` | none | removed |

Every leg line carries a 1px `--wx-casing` halo underneath (`weight + 2`,
drawn first), the established recipe for map strokes that must survive a bright
tile. `interactive: false` — the line is never a click target; the chit and the
pin are.

**Visibility modes** (`mapSettings.sagLegLines`, default `'selected'`):

| Mode | Behaviour |
|---|---|
| `'selected'` **(default)** | legs of the focused request and legs of the focused vehicle only |
| `'all'` | every active leg. Honest at 2–3 legs, spaghetti at 8; offered, not defaulted |
| `'off'` | none |

---

## 6. The assign-a-driver interaction

### The chosen design: **click the pickup → ranked candidates, on the map**

1. Operator clicks the pickup pin (or presses `d` on a focused request, or hits
   **Find a driver** on a `SagBoard` row).
2. The map enters **dispatch focus** for that request:
   - non-SAG layers dim to 30% (§4);
   - `flyToBounds` fits the pickup plus the top three candidates, using the
     existing `flyToBounds` prop and its `padding: [50,50], maxZoom: 16`;
   - the top three eligible chits gain a **rank cap** (`①②③`) and a **leader
     line** to the pickup labelled `4.2 mi ahead · ~10 min`;
   - ineligible chits stay visible, go grey, and carry their reason as a badge:
     `⊘ no seat`, `? no position`, `24m stale`, `⊘ checked out`;
   - the dock's content is replaced by `SagCandidatePanel` — the same ranking as
     a list, one row per vehicle, each with a single primary `Send`.
3. `Send` calls the existing `api.dispatchSagLeg(netId, r.id, { vehicleCheckInId,
   slotIds, allowOvercommit: false })` with **all currently waiting, unassigned
   slots** — byte-for-byte the default `SagBoard.openDispatch()` already computes.
   The existing 409 handling is reused verbatim: `seats_exceeded` renders the
   refusal card with no override button; `racks_exceeded` renders the
   confirm-and-proceed question. **Not one line of dispatch logic is
   reimplemented.**
4. When the request has more than one waiting rider, the row also shows a
   `Choose riders…` disclosure that expands the existing per-slot checkbox set.
   With one rider there is nothing to choose and no disclosure is shown.
5. `Esc` exits. Dispatch focus is **never persisted** — it is a transient mode,
   not a setting.

### Why this and not the alternatives

| Alternative | Verdict | Why it loses |
|---|---|---|
| **Drag a vehicle chit onto a pickup** | Rejected | Four independent failures. (a) On touch, marker drag fights map pan — the dominant net-control-from-a-tablet case. (b) A mis-drop dispatches the wrong van with no confirmation step and requires a `Release` with a typed reason to undo, which writes a bogus ICS-214 entry. (c) It is unreachable by keyboard, so the a11y path would have to be built anyway — and once built, it is this design. (d) **It answers the wrong question.** The operator still has to eyeball which van is closest, which is precisely the cognitive work the feature exists to remove. Drag is a mechanism for a decision already made. |
| **A bare "who's closest?" button → popup list** | Rejected as the primary | It is a strict subset of dispatch focus with the spatial check thrown away. The list says "SAG 4, 1.1 mi"; the map says "SAG 4 is 1.1 mi away *on the far side of the river with no crossing for six miles*". Putting the answer in a popover that covers the map destroys the second sentence. It survives as the `g` shortcut (§8), which *enters* dispatch focus rather than replacing it. |
| **Auto-assign / "recommended" pre-selection** | Rejected | Ranking encodes distance, seats and staleness; it cannot encode "SAG 1's driver just radioed that he's stuck behind the parade". Pre-selecting a `Send` target invites a reflex confirm on a decision the operator has context we don't. The list is ranked; nothing is preselected. [REF: automation bias] |
| **Hover-to-preview candidates** | Rejected | Hover does not exist on the primary touch target, and the strip spec's rule applies unchanged: hover may add detail, never reveal content. |
| **A separate full-screen "SAG map" route** | Rejected | Net control must not lose the stations, the weather and the course while dispatching. One map. |

### The ranking function

`rankCandidates(request, vehicles, ctx): Candidate[]` in `$lib/sagDispatch.ts`.
Pure, synchronous, table-driven-testable, no Leaflet import.

**Primary key — route distance where a route exists.**

```
vehicleChainage = resolveCandidate(
    projectOnRoute(idx, v.lat, v.lon),
    { bearingDeg: station.course, lastChainage: previousChainageFor(v), elapsedSeconds, speedMps },
    idx
).candidate?.chainageMeters
```

- `resolveCandidate` is used **with hints**, not bare: bearing first (it resolves
  the out-and-back legs on a cold start), continuity second. `previousChainageFor`
  is a small per-vehicle memo in the store, seeded on first fix.
- If `resolveCandidate` returns `ambiguous: true`, the vehicle is **not given a
  number**. It ranks below every unambiguous candidate with the badge
  `? two possible positions — could be mi 22 or mi 78`. Showing a distance here
  would be a confident lie by up to the length of the loop, which
  `routeDistance.ts` explicitly warns about.
- `forward = courseDistance(vehicleChainage, pickupChainage, idx)` and
  `reverse = courseDistance(pickupChainage, vehicleChainage, idx)`.
  - On an **open** course, rank by `min(forward, reverse)`.
  - On a **closed loop**, rank by `forward`, unless `reverse < forward / 3`, in
    which case use `reverse`.
  - **The direction word is mandatory in the UI**: `4.2 mi ahead` or
    `1.8 mi back`. A bare "1.8 mi" that silently means "backwards up a course
    full of oncoming riders" is the exact class of error this feature exists to
    prevent.

**[Q4]** Do SAG vehicles drive against the direction of travel on these
courses? Some organisers forbid it outright; some require it. If it is
forbidden, drop `reverse` entirely — the ranking simplifies and gets *more*
accurate, because a van 1 mi behind the pickup is then genuinely
`courseLength − 1` miles away.

**Off-course vehicles.** A vehicle more than `TOL_OFF_COURSE_M` (400 m) from the
line has no route distance. It gets crow-flies, **explicitly labelled and
visually distinct**: `2.1 mi direct (off course)`, with `DistanceKind: 'direct'`
— the type already exists in `routeDistance.ts` for exactly this. Off-course
candidates rank **after all on-course candidates regardless of number**, because
an unlabelled 2.1 that means something different from the 4.2 above it is
unreadable. Mixing units silently is the sin.

**ETA.** `minutes = miles / sagAvgSpeedMph × 60`, rendered `~10 min`, **minute
resolution, always with the tilde**. `sagAvgSpeedMph` is one config value
defaulting to **25**; it is not per-vehicle and it is not derived from APRS
speed (a van parked at a rest stop reports 0 mph, which would produce an
infinite ETA). Beyond 45 minutes the ETA renders `~45+ min` rather than a
precise large number. [REF: NNGroup on false precision]

**[Q5]** What is a realistic average road speed for a SAG van at these events?
25 mph is a guess. It is one config number and it drives every ETA on screen.

**Ordering, as a total sort:**

```
1. eligible (can seat the whole party)    before  partial-capacity
2. unambiguous position                   before  ambiguous
3. on-course                              before  off-course
4. fresh/aging position                   before  stale
5. shorter distance                       before  longer
```

Excluded from ranking entirely (rendered in the panel below a rule, greyed, with
a reason, **never hidden** — an absence the operator cannot explain is worse
than a greyed row): `checkInStatus === 'released'`, `availableSeats <= 0`, and
vehicles with no position at all.

**Partial capacity is a demotion, not an exclusion.** The backend model supports
partial pickup through legs and slots; a van with one seat and a two-rider
request is a legitimate answer, and its row's button reads **`Send 1 rider`**,
not `Send`. It sits below every van that can take both.

### No course loaded

Every distance degrades to crow-flies at once, and the panel says so in its
header, above the list, before any number:

```
No course loaded — these are straight-line distances, not road miles.
[ Import a course ]
```

`Import a course` reuses `navigateZone('course-import')`. In this state,
mileage-only pickups are unplaceable (`reason: 'no-course'`) and live in the
dock. The feature still works; it just refuses to pretend.

---

## 7. Selection and focus

One store, in `stores/ride.ts`:

```ts
export const sagFocus = writable<{
  requestId: string | null;
  vehicleCheckInId: string | null;
  mode: 'browse' | 'dispatch';
}>({ requestId: null, vehicleCheckInId: null, mode: 'browse' });
```

**Both directions, one rule: focus is written only on an explicit user gesture,
never inside an `$effect` that watches data.** This is what prevents the
infinite fly-to loop that two-way map/list sync usually produces.

| Gesture | Writes | Side effects |
|---|---|---|
| Click a pickup pin | `{requestId, mode:'browse'}` | `SagBoard` expands that card and scrolls it into view. On desktop, opens the side panel to the `sag` tab via the existing `openNetControl()` + `netControlRequestedTab.set('sag')` — **unless a composer or inline form is already open**, in which case the panel is left alone and only the map focus changes. Focus never steals a panel out from under a half-typed record. |
| Click a vehicle chit | `{vehicleCheckInId, mode:'browse'}` | Chit gets a selection ring; its active legs' lines draw; the `SagBoard` vehicle strip highlights that vehicle. |
| Click a `SagBoard` request row | `{requestId, mode:'browse'}` | If placeable: `flyToBounds` over pickup + dropoff + any assigned vehicle (or `flyTo` at z14 if only the pickup is placeable). **If unplaceable: no map movement at all**, and the row shows an inline note `not on the map — add a mile marker or place it` with the same two buttons as the dock card. A map that jumps to nowhere is worse than a map that stays put. |
| `Find a driver` / `d` / pin click while already focused | `mode:'dispatch'` | §6 |
| `Esc` | `mode:'browse'`, or clears focus if already browsing | — |
| Click empty map background | clears focus | Uses the existing background-click handler; inert while any pick/draw mode is active (`anyPlaceMode`). |

`SagBoard.expandedId` becomes a **derived mirror** of `$sagFocus.requestId`
rather than independent local state, so the two surfaces cannot disagree. A
`$effect` in `SagBoard` scrolls the focused card into view with
`scrollIntoView({ block: 'nearest' })` — `nearest`, so focusing an
already-visible card does not jump the list.

---

## 8. Keyboard and accessibility

### Markers are not tab stops

Every SAG marker is created with `keyboard: false`, matching the existing wx
chip markers. Thirty vans would otherwise be thirty tab stops between the map
and the panel — the same failure the strip spec rejects for its chips.

**The dock is the keyboard surface for the overlay**, built as a WAI-ARIA
toolbar, identical in pattern to the strip: **one tab stop**, roving `tabindex`,
`role="toolbar"` with `aria-label="SAG requests and vehicles"`.

| Key | Scope | Action |
|---|---|---|
| `↑` / `↓` | dock | move between cards (across groups) |
| `Home` / `End` | dock | first / last card |
| `Enter` | dock, request card | focus it, fly the map to it |
| `d` | dock, or any focused request | **enter dispatch focus** |
| `p` | dock, unplaceable card | enter `Place on map` pick mode |
| `↑` / `↓` | dispatch focus | move the highlighted candidate; the corresponding chit gets a selection ring on the map |
| `Enter` | dispatch focus | **Send** to the highlighted candidate (the over-capacity dialogs still intercept) |
| `Esc` | anywhere | exit dispatch focus, then clear focus, then close the dock |
| `g` | global | focus the **oldest request still needing a vehicle** and fly to it. The "who needs help longest" key. |

Global keys register in `+page.svelte`'s existing `handleGlobalKeydown` and
inherit the strip spec's inertness rules verbatim: dead while focus is in
`input`/`textarea`/`[contenteditable]`, while `LoginOverlay` or `SetupWizard`
is up, and while the command palette is open. `d` and `g` are free — the strip
spec claims `e n s q w c a r x ? /` and `Ctrl/Cmd+K`, none of which collide.

**No number keys for `Send`.** `1`/`2`/`3` for "send to rank N" is the obvious
design and it is wrong: ranks **reorder live** as APRS positions arrive, so the
key is bound to a moving target. A van that was `②` when the operator started
reaching for the key can be `①` by the time they press it. `↑`/`↓`/`Enter`
binds to a thing the operator is *looking at*. [DJ]

### Screen reader

- The overlay's markers are decorative to AT (they are `aria-hidden`); all
  content is in the dock and the candidate panel, which are real DOM.
- Dock: `role="toolbar"`; each card `role="group"` with `aria-labelledby`.
- Candidate panel: `role="listbox"` with `aria-activedescendant` tracking the
  highlighted candidate — the correct pattern for a list navigated by arrows
  where `Enter` commits.
- Every glyph is `<svg role="img"><title>…</title></svg>`, as `WxTierGlyph`
  already does.
- **One** `aria-live="polite"` region on the dock, carrying only: a new request
  needing a vehicle, and a dispatch confirmation. Batched to one announcement
  per 60s. **Never `assertive`** — assertive belongs solely to the interrupt
  banner, per the strip spec.
- Numbers get spoken forms, never glyph transcriptions:
  `"SAG 1, 4.2 miles ahead on the course, about 10 minutes, 2 of 4 seats free,
  4 of 4 racks free, position 4 minutes old"`.

### No status in colour alone

| State | Colour | Shape | Text |
|---|---|---|---|
| Priority tier | `--color-ride-*` | tier glyph (`RideTierGlyph`) | two-letter code |
| Needs a vehicle | tier border | **dashed outer ring** | rider numeral + `NEEDS VEHICLE` in the dock |
| Seats full | — | **closed 2px frame** + `⊘` | `no seat` |
| Racks over | — | **nub outside the frame**, hatched | `bike in the bed` |
| Position stale | muted | **dashed chit border + drift halo** | the literal word `stale` |
| Position never set | — | **absent from the map** | `position never set`, in the dock |
| Hand-placed position | — | **dotted tail to `✛`** | `set by hand at 10:42` |
| Pickup vs dropoff vs vehicle | — | **pin / pennant / chit** | labels differ |

### Motion

Exactly one animation exists: the two-cycle 1.6s pulse when a pin first appears
with `needsVehicle`. Nothing else moves, ever. The global
`prefers-reduced-motion` rule already reduces it to 0.01ms.

### Outdoor legibility

Every marker stroke carries the `--wx-casing` halo. Marker text is 11px minimum
with `font-weight: 600`. The chit's capacity bars use a 2px gap between
segments, which is the minimum that survives a sun-washed screen at arm's
length.

---

## 9. Token contract

**Four new tokens. No new hues for tiers** — every tier colour is already
aliased by the strip spec's `--color-ride-*` family and is read through
`tierStyle()`.

```css
/* SAG map overlay. --color-sag-unit is NOT a new hue: it is the value
   stationCategoryMeta.ts already assigns to the `sag` check-in category
   (#f97316), promoted to a token so the map and the roster cannot drift.
   Contrast, computed against the three surfaces:
     #f97316 on --color-surface #16213e -> 5.67:1   pass AA as text
     #f97316 on --color-bg      #1a1a2e -> 6.09:1   pass AA as text
   Text-safe, so the chit's label may be drawn in it. */
--color-sag-unit: #f97316;
--color-sag-unit-soft: rgba(249, 115, 22, 0.14);
/* The stale-position uncertainty disc. Fill only, never a text colour. */
--color-sag-drift: rgba(249, 115, 22, 0.10);
/* Pane z-indices, read the way readWxTokens() reads --z-wx-pane. */
--z-sag-link-pane: 420;
--z-sag-marker-pane: 620;
```

Dash patterns join the existing `--wx-dash-*` family for consistency:

```css
--sag-dash-leg-dispatched: 6 5;    /* told, not yet rolling */
--sag-dash-pickup-dropoff: 2 7;    /* the quietest dash in the app */
```

Marker geometry, so the dock and the map cannot disagree about hit targets:

```css
--sag-chit-w: 56px;  --sag-chit-h: 28px;
--sag-pin-w: 32px;   --sag-pin-h: 38px;
--sag-hit: 44px;     /* every marker's transparent hit padding */
```

Everything else is existing tokens: `--color-ride-*` and their `-text` variants,
`--color-text`, `--color-text-muted`, `--color-surface`, `--color-warning`,
`--wx-casing`, `--space-xs/sm/md`, `--radius-sm`, `--shadow-md`,
`--duration-fast`, `--ease-out`, `--z-toolbar`, and the strip spec's
`--ride-t-value/-body/-label` type scale.

**Reused, not redefined:** `--color-sag-dropoff` is not a token —
dropoff pennants use `--color-text-muted` directly. One fewer indirection for a
value that will never need to vary independently.

### `:global` requirement

Every marker is injected through `L.divIcon`, so its markup is **outside the
Svelte component tree** and scoped styles will not reach it. All marker CSS
lives in one `:global(...)` block, and the tokens above must resolve on an
ancestor of the Leaflet container — they are declared on `:root` in `app.css`,
so they do. Precedent: the existing `.net-voice-marker` / `.net-mission-flag`
classes, which pass colours inline for exactly this reason. **The SAG markers go
the other way: classes plus `:global` CSS, no inline colours**, because the
capacity bars need `::before`/`::after` and hatch fills that cannot be expressed
inline. This is a deliberate divergence from the existing markers and should be
noted in the implementation.

---

## 10. Empty and degraded states

| Condition | Map | Dock | Candidate panel |
|---|---|---|---|
| Not ride mode | overlay never mounts | not rendered | — |
| Ride mode, no SAG board yet | nothing | `Loading SAG…` | — |
| No SAG requests at all | nothing | `Nobody is waiting for a ride.` (muted, **not green** — absence is not verified good) | — |
| No SAG **vehicles** checked in | vehicle layer empty | **`⚠ No SAG units checked in`** in `--color-warning` with `Check in a unit with category "sag"`. An absence that matters. [strip spec B5] | replaced by that same message |
| No vehicle has a position | pickups draw; no chits | `🚐 NO POSITION (n)` group listing each, each with `Place on map` | header: `No SAG unit has a position — ranking by distance is not possible. Place them on the map, or dispatch from the board.` The list still renders, unranked, alphabetical, still with `Send`. |
| No course loaded | mileage pickups do not draw; coordinate/annotation pickups do | `⚑ NOT ON THE MAP` group with `no-course` copy + `Import a course` | `No course loaded — these are straight-line distances, not road miles.` + Import button |
| Pickup unplaceable | absent from the map | its card in `⚑ NOT ON THE MAP`, above every other group | dispatch focus still works: the candidate list renders **unranked**, with `distance unknown — the pickup isn't on the map` |
| Every vehicle stale | chits drawn dashed with drift halos | each card carries `stale` | ranked, every row carries `stale`, header adds `Every SAG position is more than 20 minutes old.` |
| Course is a loop | — | — | distances wrap (`courseDistance` already handles it); direction word still mandatory |
| Request has zero waiting riders (all assigned/resolved) | pin loses its ring and numeral | moves out of `⬒ WAITING` into a collapsed `in motion (n)` group | not offered as a dispatch target |

**The governing rule, from the strip spec, applied here unchanged:** a
never-reported value never renders as `0`, and a stale value is never painted
green. [REF: Grafana's explicit NoData state]

---

## 11. Layer toggle, defaults and responsive

### `MapPalette` additions — a section, gated on ride mode

The whole section renders only when `$rideMode && $sagBoard`. A search-and-
rescue net never sees a SAG control.

```
┌ SAG ──────────────────────────────┐
│ [x] SAG overlay           3 open  │   mapSettings.showSagOverlay   default TRUE
│ Leg lines  [ Selected ▾ ]         │   mapSettings.sagLegLines      default 'selected'
│                                   │       Selected · All · Off
│ [x] SAG dock                      │   mapSettings.showSagDock      default TRUE
│ [ SAG focus ]      ← one-click    │   dims everything else (§4)
└───────────────────────────────────┘
```

- **On by default, in ride mode only.** A ride net exists to run SAG; asking the
  operator to turn it on is asking them to discover it during the first
  emergency. Non-ride nets are unaffected because the layer does not mount.
- Persisted in the existing `mapSettings` localStorage blob; `hasNonDefault`
  gains `showSagOverlay === false || sagLegLines !== 'selected'` so the
  indicator dot correctly warns that SAG is hidden.
- **Dispatch focus is never persisted.** It is a mode, not a setting.

### Breakpoints

Reuse `app.css`'s existing two queries verbatim. Do not invent a third system.

| Viewport | Treatment |
|---|---|
| **>=1200w, >=500h** | Dock at the map's left edge, 260px, collapsible to a 32px stub. Markers at desktop sizes. Dispatch focus replaces the dock's content in place. |
| **769–1199w, >=500h** | Dock collapses to a **44px vertical rail** of tier-coloured stubs with counts (`PR 2`, `HI 1`, `⚑ 1`); clicking expands it as a 260px overlay above the map. Markers at **touch sizes** — a 1024px tablet is a touch device. |
| **<=768w** | **No dock.** Markers at touch sizes (pin 40×46, chit 64×32, 44px hit padding). `BottomSheet` gains a `sag` mode carrying the dock's content. Dispatch focus snaps the sheet to `half` and renders the candidate list at >=56px per row; the map's `flyToBounds` padding is increased by the sheet's height so the pickup is never behind the sheet — the same arithmetic `--sheet-peek` already drives. |
| **<=499h (landscape phone)** | No dock (matching `.desktop-only`). Markers still render — in landscape the map *is* the whole UI. Dispatch focus opens the candidate list as a full-height scrim sheet over the right third. |

The SAG layer does **not** change the map's box. `.map-layer`'s inset is already
owned by `--ride-strip-h`; the dock is an absolutely-positioned overlay inside
the map layer, like `MapPalette` and `GpsStatusPill`. **No `invalidateSize()`
call is needed for anything in this spec** — and if an implementer finds
themselves adding one, the dock has been built as a layout sibling by mistake.

---

## 12. Anti-recommendations

The considered-and-rejected list. This is the part to read before adding anything.

| Rejected | Why |
|---|---|
| **Drag-and-drop dispatch** | Four failures (§6): fights map pan on touch, no confirmation on mis-drop, unreachable by keyboard, and it answers the wrong question — the operator still has to eyeball distance. |
| **Isochrone / drive-time coverage shading** | It is the most beautiful version of this feature and we cannot build it honestly. A real isochrone needs a routing engine with road geometry and speed limits; approximating one from the course line produces a confident picture that is wrong wherever a road leaves the course — which is everywhere a SAG van actually drives. |
| **Coverage heatmap ("where are we thin on SAG?")** | Unactionable mid-ride. NCS cannot conjure a van into a gap. It is a pre-ride planning artefact and belongs in a pre-ride view, if anywhere. |
| **Auto-dispatch, or pre-selecting the top candidate** | The ranking knows distance, seats and staleness. It does not know that SAG 1's driver just radioed that he is stuck behind a parade. Pre-selection invites a reflex confirm on a decision the operator has context for and we do not. Rank, never choose. [REF: automation bias] |
| **Number keys (`1`/`2`/`3`) to send** | Ranks reorder live as positions arrive; the key binds to a moving target. `↑`/`↓`/`Enter` binds to the row the operator is looking at. |
| **Clustering pickup pins** | Clustering hides the count that matters at exactly the zoom where a cluster forms — "3 riders at mile 34" becomes "a circle with a 3 in it" that means something different. Pickups are few (single digits) and each one is a person on a roadside. Never cluster them. |
| **Leaflet popups on markers** | A popup covers the neighbouring markers the operator is comparing against — the exact information the map click was trying to surface. Detail goes to the dock and the panel, which sit beside the map, not on it. Tooltips (hover/focus, non-modal, small) are fine. |
| **Bibs on the map** | Bibs are labels on exceptions, never keys [DOM fact #1], and putting one on a pin implies a roster that does not exist. It is also a privacy call the organiser may have made the other way [DOM fact #7]. The pin shows a *count*. |
| **Rider names on the map** | Same, harder. Names exist only for hospital/start transports [DOM fact #7] and must never leak onto a shared screen. |
| **Rotating the vehicle chit by APRS course** | Heading tells you which way a van is pointed, not which way it is going to drive. A van pointed north at a rest stop is about to do whatever it is told. Rotation would be read as intent. |
| **Animated vehicle trails / breadcrumb tails** | The station track layer already does this and can be turned on. A second, prettier version competing at the same z-index is how the map becomes unreadable. |
| **Drawing every leg line, always** | Honest at 2 legs, spaghetti at 8, and the 8-leg case is the one where the map has to work. Offered as `sagLegLines: 'all'`; never the default. |
| **Crow-flies distance without a label** | The single most dangerous thing in this feature. A number with no unit-of-meaning is worse than no number. Every distance says `ahead`, `back`, or `direct (off course)`. |
| **ETA as a clock time (`arrives 11:42`)** | False precision on a number built from a guessed average speed. `~10 min` is honest about being an estimate; `11:42` is not. [REF: NNGroup] |
| **A map legend** | The strip spec already rejected this and the reasoning is unchanged: a legend is an admission the symbols failed. Three silhouettes, two-letter codes, tooltips with words. |
| **A separate full-screen SAG map route** | Net control must not lose the stations, the weather and the course in order to dispatch. One map. |
| **Showing dropoff pennants for every request at once** | Dropoffs are usually rest stops, which are already on the map as checkpoints. All of them at once is a field of grey pennants sitting on top of the stops they duplicate. Focused request only. |
| **A "SAG" base-map style / colour theme** | Theming the whole map for one layer breaks every other layer's colour contract. Dimming, not restyling. |
| **Encoding load as a single "fullness" colour (green/amber/red)** | Collapses two asymmetric dimensions into one, loses the count, loses the seats-vs-racks distinction that is the entire operational nuance, and fails in greyscale. Two segmented bars. |
| **Persisting dispatch focus across reloads** | It is a mode the operator is *in*, not a setting they *have*. Restoring it on reload would put a returning operator inside a dimmed map with no memory of why. |

---

## 13. Build order

Sequenced for one implementer, **riskiest and most uncertain first**, per project
law (TDD for anything with arithmetic in it).

| # | Work | Risk | Done when |
|---|---|---|---|
| **1** | **`pointAtChainage()` in `routeDistance.ts`, tests first.** Table-driven: start, end, midpoint, exact vertex, out-of-range on an open course, wrap on a loop, and the round-trip property against both real fixtures. | **Highest.** Every pin on the map is a lie if this is wrong, and a plausible-looking wrong pin will be believed and acted on. | `make test` green; a mile-34 pin on `GR_2025_100_miler.kml` lands where the real mile 34 is. |
| **2** | **`$lib/sagGeo.ts` — `resolveSagPoint()` + the unplaceable taxonomy.** Tests for all six resolution rules, all four unplaceable reasons, the multi-route case, and the measured-vs-configured distance mismatch. Pure, no Svelte. | High — this is where §2's [Q1]/[Q2] bite. Land the code with the questions answered or with the conservative branch (refuse, don't guess). | Every `SAGLocation` shape in the codebase's fixtures resolves to a `SagPlacement` with the right `via` or the right `reason`. |
| **3** | **`$lib/sagDispatch.ts` — `rankCandidates()`.** Tests for: on-course ordering, loop wrap, reverse-direction, off-course fallback to `direct`, ambiguous projection (no number emitted), stale demotion, partial-capacity demotion, seats-full exclusion, released exclusion, no-course total degradation. Pure, no Leaflet. | High — the ordering rules are the product. | A table test asserts the exact §6 sort order across a fixture of 8 vehicles. |
| **4** | **Derived stores in `stores/ride.ts`:** `sagPlacements`, `sagUnplaced`, `sagVehiclePositions` (joining `SAGVehicleStatus.checkInId` → `NetCheckIn`), `sagFocus`. No UI yet. | Medium — the check-in join is where a missing `checkInId` silently drops a van. | Svelte devtools shows correct buckets against a live net. |
| **5** | **Map overlay, static.** Two panes, the three marker `$effect`s, the `:global` marker CSS, chit-replaces-station-marker, `keyboard: false`, casing halos. No clicking, no lines, no focus. | Medium — this is where the ninth-layer clutter problem becomes visible and the §4 suppression table gets validated against a real busy map. | Screenshot on a real course at three zooms; all three silhouettes distinguishable in greyscale. |
| **6** | **Lines** (§5), both kinds, with the leg-status retargeting. | Low | Advancing a leg through `dispatched → enroute → loaded` visibly swings the line to the dropoff. |
| **7** | **`sagFocus` two-way sync** with `SagBoard` (§7), including the no-map-movement-for-unplaceable rule and the don't-steal-an-open-composer rule. | Medium — the classic infinite-loop bug lives here. | Clicking back and forth twenty times produces no runaway `flyTo`. |
| **8** | **`SagDock.svelte`,** all four groups, the unplaceable cards, and `Place on map` wired through the existing `commitPick`/`PickSource` path. | Low-medium | An unplaceable request can be placed with two clicks and no typing. |
| **9** | **Dispatch focus + `SagCandidatePanel.svelte`,** including rank caps, leader lines, the dim class, and `Send` calling the **existing** `api.dispatchSagLeg` with the **existing** 409 handling copied from `SagBoard`. | Low — the hard parts are already in steps 3 and 8. | End-to-end: radio call → pin → `d` → `Enter` → dispatched, in under ten seconds. |
| **10** | **`MapPalette` section + `SAG focus` preset** with its settings snapshot/restore. | Low | Toggling off and on restores the operator's prior filter state exactly. |
| **11** | **Keyboard + a11y pass** (§8): dock toolbar, roving tabindex, `d`/`g` globals with the inertness guards, listbox semantics, one polite live region, spoken number forms. | Low, but do not skip — CLAUDE.md frontend law. | Whole flow completed with no mouse; NVDA/VoiceOver reads distances as sentences. |
| **12** | **Responsive** (§11): tablet rail, phone bottom-sheet mode, touch marker sizes, fit-padding arithmetic. | Low-medium — the sheet-aware `flyToBounds` padding is fiddly. | Dispatch completed on a 390px phone without the pickup ever hiding behind the sheet. |
| **13** | **Wiki + roadmap.** Check off the WP7 SAG map item in `wiki/Home.md`, document what works. Submodule push order: `cd wiki && git push origin master` **first**, then the main repo. | — | — |

Steps 1–3 are pure functions with tests and no UI. **If they are not landed and
green before step 5, stop** — every later step inherits their errors as pixels
on a map an operator will trust.

---

## 14. Open questions

Collected from the body, in the order they change the most work. None of these
were guessed at in the design; each has a conservative default that the spec
takes until answered.

| # | Question | Spec's interim behaviour | What changes if answered |
|---|---|---|---|
| **Q1** | Do your events run multiple route annotations inside one net, or one net per route? | Resolver refuses (`route-unknown`) when several routes exist and `loc.route` is empty. | One-net-per-route kills the `route-unknown` branch and simplifies §2. |
| **Q2** | When a rest stop says "mile 34", is that the **painted mile marker** (advertised distance) or their **GPS read** (measured distance)? | Mileage is converted against the measured GPX length; a >2% mismatch shows a notice. | If it is the painted marker, mileage must be scaled by `configDistance / measuredDistance` and the notice inverts into a correction. Directly affects every pin's accuracy. |
| **Q3** | Realistic top road speed for a SAG van on your courses — is 35 mph a sane cap for the stale-position drift halo? | 35 mph, capped at 8 mi. | Highway running between rest stops → 55 mph and a much larger halo. |
| **Q4** | Do SAG vehicles drive **against** the course direction? Forbidden, permitted, or required in some cases? | Both directions are considered; the direction word (`ahead` / `back`) is always shown. | If forbidden, drop `reverse` entirely — ranking gets simpler **and more accurate**, because a van 1 mi behind is then genuinely a lap away. |
| **Q5** | Realistic average road speed for ETA purposes? | One config value, default 25 mph, rendered `~N min`. | One number; it drives every ETA on screen. |
| **Q6** | Should a `hospital` dropoff take the vehicle **out of the available pool** for a configurable period? | No — it stays rankable; only its seats/racks change. | If yes, hospital-bound vans get an `off the course` badge and demote below everyone, which meaningfully changes ranking during the worst part of a ride. |
| **Q7** | Is there ever more than one SAG vehicle from the same check-in (a van plus a trailer), or is `checkInId` genuinely one-vehicle-one-chit? | One `checkInId` → one chit. | A second vehicle per check-in would break the chit-replaces-station-marker rule in §3.3. |

---

## Sources

**Domain ground truth** (from `docs/ride-mode-plan.md`, established and not
re-litigated here): edge-based rider accounting; route-relative position as the
primary coordinate system; agency-configurable priority vocabulary; bibs as
labels on exceptions, never keys; privacy rules on bibs and names; SAG request
structure with slots and legs; the seats-refuse / racks-ask capacity asymmetry
(from `internal/ride` and `SagBoard.svelte`'s existing dialogs).

**Codebase precedent:** `Map.svelte`'s `$effect`-per-overlay pattern, the
`wxAlertPane` + `readWxTokens()` recipe, `layerClick`/`commitPick`'s single
choke point for pick modes, `flyToTarget`/`flyToBounds`, the
`--wx-casing` halo for map strokes, `BottomSheet`'s `--sheet-peek`
runtime-published height, `MapPalette`'s disabled-with-a-reason toggles,
`routeDistance.ts`'s `projectOnRoute`/`resolveCandidate`/`courseDistance` and
its documented out-and-back ambiguity, `stationCategoryMeta.ts`'s `sag` colour,
`rideMeta.ts`'s `ageState`/`tierStyle`/`unassignedRiders`,
`docs/ride-strip-spec.md`'s staleness vocabulary, tier-colour contrast rules,
toolbar keyboard pattern and one-polite-live-region discipline.

**External references:** RAG status (colour never the only channel) · Grafana's
explicit NoData state, distinct from Normal and Alerting · NNGroup on false
precision in progress and time estimates · automation bias in decision-support
systems · dark cockpit / annunciator discipline (render only what is true).

**Explicitly not verified and flagged as questions:** SAG vehicle road speeds,
course-direction driving policy, mile-marker provenance, and hospital-transport
pool effects (§14). These are operational facts about the user's events and are
deliberately left as [Q] rather than invented.
