# Bike-ride dashboard — design pass

Status: spec only. No `.svelte` / `.ts` / `.css` file was edited to produce this.

**The complaint being chased:** *"The titles are not vertically constant and
generally messy looking."* §2 proves it with an inventory. §3 gives the one
recipe each element type should follow. §4 is the mechanical change list.

**Scope:** `web/src/lib/components/ride/*.svelte`, `web/src/lib/components/Ride*.svelte`,
`web/src/app.css`. `NetControlPanel.svelte` is the **reference house style** and is
not itself critiqued.

> **Line-number caveat.** `SagRequestComposer.svelte`, `SupplyComposer.svelte` and
> `MedicalComposer.svelte` were being edited by another process while this pass was
> written. Their line numbers below may have drifted by a few lines; the **selector
> names** are authoritative. Every other file's line numbers are exact as of this
> pass.

---

## §1 Token reality — what `app.css` actually offers

Read from `/home/narvel/dev/Nymeria/web/src/app.css`.

### Spacing (app.css:9–18) — the complete scale, 7 steps

| Token | Value | Comment in source |
|---|---|---|
| `--space-2xs` | `2px` | app.css:12 — "the inside-a-chip measure (glyph-to-code, icon-to-count)" |
| `--space-xs` | `4px` | app.css:13 |
| `--space-sm` | `8px` | app.css:14 |
| `--space-md` | `16px` | app.css:15 |
| `--space-lg` | `24px` | app.css:16 |
| `--space-xl` | `32px` | app.css:17 |
| `--space-2xl` | `48px` | app.css:18 |

**There is no 3px, 5px, 6px or 10px step.** Every one of those that appears in a
ride component is off-scale. §4 lists 45 of them.

### Radius (app.css:20–24)

`--radius-sm: 6px` · `--radius-md: 8px` · `--radius-lg: 12px` · `--radius-full: 9999px`

### Type — there is **no global type scale**

app.css:176–182 says so in as many words:

```
/* Type roles for the ride strip. app.css has no type scale and components
   hard-code rem sizes; the strip has eight tiles that must agree, so
   these three are named by ROLE. */
--ride-t-value: 1.375rem;   /* 22px */
--ride-t-body:  0.875rem;   /* 14px */
--ride-t-label: 0.6875rem;  /* 11px */
```

These three are the **only** type tokens in the codebase. They were introduced
for `RideStrip`/`RideZone`/`RidePeek` and were then adopted by `SagDock.svelte`
and `SagCandidatePanel.svelte` — and by nothing else in ride mode. Every other
ride component hard-codes rem values. That split is the root cause of the
complaint: **two families of ride components, one tokenized and one not, render
side by side in the same panel.**

### Colour tokens relevant here

- Surfaces: `--color-bg #1a1a2e`, `--color-surface #16213e`, `--color-primary #0f3460`
  (used throughout as the 1px border colour), `--color-raised`, `--color-raised-strong`,
  `--color-hairline`.
- Text: `--color-text #eee`, `--color-text-muted #aaa`.
- Semantic: `--color-accent #e94560`, `--color-success`, `--color-warning`,
  `--color-error`, `--color-info`, `--color-on-accent`, `--color-on-warning`, `--color-scrim`.
- **Text-safe variants (app.css:63–74, 101–118, 153–163).** The rule is stated in
  the file: base tier tokens are for *glyph fills, borders and rules* (≥3:1); the
  moment a colour tints **a word or a numeral**, the `-text` variant is required.
  `--color-error-text`, `--color-info-text`, `--color-ride-emergency-text`,
  `--color-ride-medium-text`, `--color-wx-*-text`.
- **Ride priority ladder (app.css:139–165).** Keyed by **rank**, never by name —
  the five tier names are agency data from `GET /nets/{id}/profile`. Components
  reach them only through `tierStyle()` in `lib/rideMeta.ts:35`, which returns
  `colorVar` / `textVar` / `softVar` strings. **No spec in §3 hardcodes a tier.**
- SAG identity: `--color-sag-unit #f97316` (app.css:194) — documented as text-safe
  (5.67:1 on surface, 6.09:1 on bg).

### Geometry tokens (app.css:213–218)

`--sag-chit-w/h`, `--sag-pin-w/h`, and **`--sag-hit: 44px`** — a touch-target token
that already exists and that **no ride panel currently uses.** See §5.

**Audit result:** the ride family uses **19 distinct literal font sizes** (0.625,
0.64, 0.65, 0.68, 0.7, 0.72, 0.75, 0.78, 0.8, 0.82, 0.85, 0.9, 0.95, 1.05, 1.2,
1.4 rem and 9px, 10px, 12px) **plus** the 3 role tokens. And **6 distinct gap
values** (2, 3, 4, 5, 6, 8 px) on a scale that offers 2, 4, 8.

---

## §2 The title inventory — the evidence

Every section-, panel-, card- and dialog-title in scope. Sorted by role.

### 2.1 Small-caps section headers / eyebrows / field legends

*These all do the same job: name the block below them.*

| # | File:line | Class | Element | Size | Weight | Transform | Tracking | Colour | Box |
|---|---|---|---|---|---|---|---|---|---|
| 1 | SagBoard:1260 | `.sb-section-h` | `<h4>` | `0.68rem` | 700 | uppercase | `0.04em` | muted | `margin: 4px 0 0` |
| 2 | SagBoard:1199 | `.sb-detail-label` | `<span>` | `0.65rem` | **400** | uppercase | `0.04em` | muted | — |
| 3 | SagBoard:1413 | `.sb-inline-label` | `<span>` | `0.68rem` | **400** | uppercase | **none** | muted | — |
| 4 | SagBoard:1031 | `.sb-attach-label` | `<span>` | `0.72rem` | 700 | uppercase | `0.04em` | muted | — |
| 5 | SagDock:710 | `.sd-group` | `<h3>` | `var(--ride-t-label)` | **800** | **none** (text pre-uppercased in markup, SagDock:385/437/514) | `0.08em` | muted | `margin-top: 4px` |
| 6 | SagDock:724 | `.sd-group--toggle` | **`<div>`** | same | 800 | none — **and the content is lowercase** (`in motion`, SagDock:484) | `0.08em` | muted | `padding: 2px 0` |
| 7 | SagDock:583 | `.sd-rail-title` | `<span>` | **`9px`** | 800 | none | `0.08em` | `--sag-unit` | — |
| 8 | SagCandidatePanel:672 | `.cp-kicker` | `<span>` | `var(--ride-t-label)` | 800 | none — **content is `CANDIDATES for`, mixed case** (SagCandidatePanel:369) | `0.08em` | muted | — |
| 9 | RideSituation:127 | `.rs-heading` | `<h3>` | `0.7rem` | 700 | uppercase | `0.06em` | muted | header `padding: 10px 16px 6px` |
| 10 | RideSituation:207 | `.rs-closeout-heading` | `<h4>` | `0.7rem` | 700 | uppercase | **none** | muted | `margin-bottom: 6px` |
| 11 | RideSituation:156 | `.rs-row-label` | `<span>` | `0.65rem` | 700 | uppercase | **none** | muted | `width: 88px` |
| 12 | RideZone:105 | `.ride-zone-label` | `<span>` | `var(--ride-t-label)` | 700 | uppercase | `0.06em` | muted | `line-height: 1.15` |
| 13 | RideConfigSheet:244 | `.rcs-title` | `<h3>` | `0.7rem` | 700 | uppercase | **`0.03em`** | muted | — |
| 14 | SagLocationField:170 | `.slf-legend` | `<legend>` | `0.7rem` | 700 | uppercase | `0.04em` | muted | `padding: 0 4px` |
| 15 | SagLocationField:192 | `.slf-label` | `<span>` | `0.68rem` | **400** | **none** | muted | — |
| 16 | SagRequestComposer:350 | `.src-label` | `<span>` | `0.7rem` | 700 | uppercase | `0.04em` | muted | — |
| 17 | SagRequestComposer:410 | `.src-legend` | `<legend>` | `0.7rem` | 700 | uppercase | `0.04em` | muted | `padding: 0 4px` |
| 18 | SupplyComposer:353 | `.suc-label` | `<span>` | `0.7rem` | 700 | uppercase | `0.04em` | muted | — |
| 19 | SupplyComposer:413 | `.suc-legend` | `<legend>` | `0.7rem` | 700 | uppercase | `0.04em` | muted | `padding: 0 4px` |
| 20 | MedicalComposer:290 | `.mec-label` | `<span>` | `0.7rem` | 700 | uppercase | `0.04em` | muted | — |
| — | **NetControlPanel:4464** | **`.summary-kicker`** | — | **`0.65rem`** | **700** | **uppercase** | **`0.05em`** | **muted** | **— (house style)** |
| — | NetControlPanel:4492 | `.summary-label` | — | `0.65rem` | 400 | uppercase | `0.05em` | muted | — |
| — | NetControlPanel:4646 | `.metric-label` | — | `0.6rem` | 400 | uppercase | `0.05em` | `opacity: .7` | — |

**Diagnosis — one job, 20 renderings.**

- **Six sizes** for one role: `9px`, `0.65rem`, `0.68rem`, `0.6875rem` (token),
  `0.7rem`, `0.72rem`. Rows 1/3 and 2 differ by 0.03rem — half a pixel — which is
  exactly the kind of difference that reads as "sloppy" without being nameable.
- **Three weights**: 400 (rows 2, 3, 15), 700 (10 rows), 800 (rows 5–8).
- **Five tracking values**: none, `0.03em`, `0.04em`, `0.06em`, `0.08em`.
- **Casing applied three different ways**: by CSS `text-transform` (rows 1–4, 9–17),
  by literal uppercase in the markup with no `text-transform` (row 5:
  `⚑ NOT ON THE MAP`, `⬒ WAITING`, `🚐 NO POSITION`), and **not at all** (row 6
  `in motion`, row 8 `CANDIDATES for`). Rows 5 and 6 share the class `.sd-group`
  and render with different casing 50 lines apart.
- **Two element types for the same visual**: `<h3>` at SagDock:385/437/514 but a
  `<div role="button">` at SagDock:476. The `<div>` also carries `padding: 2px 0`
  that the `<h3>` does not, so the "in motion" header sits **4px lower** than the
  "WAITING" header above it. That is a literal, measurable instance of "titles are
  not vertically constant."

### 2.2 Panel / dock headers — the vertical-jump bug

| File:line | Class | Padding | `align-items` | Title size/weight | Computed header height |
|---|---|---|---|---|---|
| SagDock:642 | `.sd-head` | `var(--space-sm) var(--space-sm) var(--space-xs)` = **8 / 8 / 4** | **`baseline`** (SagDock:644) | `.sd-title` `--ride-t-body` / **800** (SagDock:650) | ≈ **27px** |
| SagCandidatePanel:653 | `.cp-head` | `var(--space-sm)` = **8 all round** | `center` (via `.cp-head-row`, SagCandidatePanel:661) | `.cp-kicker` `--ride-t-label` / 800 + `.cp-name` `--ride-t-body` / 800 | ≈ **46px** |
| RideSituation:120 | `.rs-header` | **`10px 16px 6px`** | `center` | `.rs-heading` `0.7rem` / 700 | ≈ 31px |
| NetControlPanel:4045 | `.panel-header` | `var(--space-sm) var(--space-md)` | `flex-start` | `.title` `0.95rem` / 600 | — |

**This is the headline finding.** `SagCandidatePanel` **replaces** `SagDock`'s
content in the same slot the instant the operator presses `d` / "Find a driver"
(`startDispatchFocus`, SagDock:170/466). The two headers are 19px different in
height and use different `align-items`. The panel title therefore *moves
vertically* at the exact moment the operator is reaching for a Send button. That
is the user's complaint, verbatim, and it is a dispatch-safety issue as well as a
cosmetic one.

Second offender: `RideSituation:124` uses `padding: 10px var(--space-md) 6px`.
`10px` and `6px` are both off-scale, and no other header in ride mode uses either.

### 2.3 Dialog titles

| File:line | Class | Element | Size | Weight |
|---|---|---|---|---|
| SagRequestComposer:328 | `.src-title` | `<h2>` | `1.05rem` | 700 |
| SupplyComposer:336 | `.suc-title` | `<h2>` | `1.05rem` | 700 |
| MedicalComposer:273 | `.mec-title` | `<h2>` | `1.05rem` | 700 |
| RidePendingList:164 | `.rpl-title` | `<h2>` | `1.05rem` | 700 |
| **RideConfigSheet:240** | **`.rcs-h2`** | `<h2>` | **`0.9rem`** | 700 | ← outlier |
| — NetControlPanel:4322 | `.cn-title` | — | `0.9rem` | **600** (house) |
| — NetControlPanel:4067 | `.title` | — | `0.95rem` | 600 (house) |

The four composers/dialogs are **perfectly consistent with each other** — and
every one of them is **heavier and larger than the house style**
(`1.05rem/700` vs `0.9rem/600`). `RideConfigSheet` matches the house size but not
the house weight. So there are effectively three dialog-title treatments in play.

### 2.4 Card titles (the strongest text in a list row)

| File:line | Class | Size | Weight | Colour |
|---|---|---|---|---|
| SagBoard:1134 | `.sb-seq` (`SAG 7`) | `0.82rem` | 700 | inherit |
| SupplyBoard:296 | `.sup-from` (callsign) | inherits `0.8rem` (`.sup-row`, SupplyBoard:276) | 700 | inherit |
| MedicalBoard:330 | `.med-who` | inherits `0.8rem` (`.med-row`, MedicalBoard:320) | 700 | inherit |
| SagDock:797 | `.sd-name` (`SAG 7`) | inherits `--ride-t-body` = `0.875rem` (SagDock:779) | 700 | `--color-text` |
| SagCandidatePanel:772 | `.cp-unit` | inherits `--ride-t-body` (SagCandidatePanel:763) | **800** | `--sag-unit` |
| SagDock:803 | `.sd-unit` | inherits `--ride-t-body` | 700 | `--sag-unit` |

**`SAG 7` renders at `0.82rem/700` on the board and `0.875rem/700` on the dock** —
the same string, the same net, two surfaces the operator flips between, two sizes.

### 2.5 Warning / refusal heads — same component, two weights

| File:line | Class | Weight |
|---|---|---|
| SagBoard:1465 | `.sb-refusal-head`, `.sb-overcap-head` | **700** |
| SagCandidatePanel:909 | `.cp-refusal-head`, `.cp-overcap-head` | **800** |
| SagDock:696 | `.sd-warn-head` | 700 |
| SagCandidatePanel:707 | `.cp-warn-head` | 700 |

`SagCandidatePanel:189–193` explicitly says its refusal/overcap blocks are
`SagBoard.svelte`'s, copied. The copy drifted one weight step.

---

## §3 The canonical spec

Every recipe below is derived from `NetControlPanel.svelte`, cited. Nothing is
invented. All values are tokens or token arithmetic.

### 3.0 One token addition (the only new token this spec asks for)

`app.css` currently has no `700`/`800` weight tokens and no tracking token, and
the 6-value tracking spread is half the mess. Add to `:root`, beside the existing
`--ride-t-*` block (app.css:176–182):

```css
/* One tracking value for every small-caps label in ride mode. Matches
   NetControlPanel's .summary-kicker / .summary-label / .metric-label,
   which are the house style (NetControlPanel.svelte:4464, 4492, 4646). */
--ride-label-tracking: 0.05em;
```

`--ride-t-label` (`0.6875rem`, app.css:182) becomes the single section-header size.
It is within 0.6px of the house `0.65rem`, it already exists, and it is already
used by `RideZone`, `SagDock` and `SagCandidatePanel` — adopting it removes six
literals instead of introducing a seventh.

### 3.1 THE section-header recipe

*Derived from `NetControlPanel.svelte:4464` `.summary-kicker`.*
*Applies to: every eyebrow, group header, fieldset legend and field label that
names the block beneath it.*

```css
/* ride: section header / eyebrow / legend / field label */
font-size: var(--ride-t-label);          /* 0.6875rem — the one label size */
font-weight: 700;                        /* never 400, never 800 */
text-transform: uppercase;               /* casing in CSS, never in the markup */
letter-spacing: var(--ride-label-tracking);
color: var(--color-text-muted);
line-height: 1.2;
margin: 0;                               /* spacing comes from the parent's gap */
```

Rules that come with it:

1. **Casing is a CSS concern.** No component writes `NOT ON THE MAP` or
   `CANDIDATES for` into the template. Markup carries sentence case; CSS uppercases.
2. **Always `<h3>` or `<h4>`** (or `<legend>` inside a `<fieldset>`), never a
   `<span>` or a bare `<div>`, and never both for the same visual in one file.
3. **No margin on the header.** Vertical rhythm comes from the parent flex
   container's `gap`. This is what makes the height constant.
4. A count suffix uses the chip recipe (§3.4), not an inline `opacity`.

### 3.2 THE panel/dock header recipe

*Derived from `NetControlPanel.svelte:4045` `.panel-header`.*
*Applies to `.sd-head`, `.cp-head`, `.rs-header`, `.rcs-header`, `.rpl-head`,
and the composer title rows.*

```css
/* ride: panel header — a FIXED 44px band so swapping panels never moves it */
display: flex;
align-items: center;                     /* never `baseline` */
gap: var(--space-sm);
min-height: 44px;                        /* = var(--sag-hit); the constant */
padding: 0 var(--space-sm);              /* horizontal only; min-height owns vertical */
border-bottom: 1px solid var(--color-primary);
flex-shrink: 0;
```

with the title inside it:

```css
/* ride: panel title */
font-size: var(--ride-t-body);           /* 0.875rem */
font-weight: 700;
letter-spacing: 0.02em;
/* colour: --color-text, or --color-sag-unit where the panel IS the SAG identity */
```

A sub-line (the candidate panel's location · riders · age row) goes **below** the
44px band as its own row, not inside it — that is what keeps the title's baseline
constant whichever panel is mounted.

### 3.3 THE card recipe

*Derived from `NetControlPanel.svelte:4795` `.mission-card` + `:4453` `.summary-card`.*
*Applies to `.sb-card`, `.sup-card`, `.med-card`, `.sd-card`, `.cp-row`, `.sb-leg`, `.rpl-row`.*

```css
/* ride: card */
display: flex;
flex-direction: column;
gap: var(--space-xs);
padding: var(--space-sm);
background: var(--color-surface);
border: 1px solid var(--color-primary);
border-left: 3px solid var(--ride-card-tier, var(--color-primary));
border-radius: var(--radius-sm);
flex-shrink: 0;                          /* SagDock:749 / SagCandidatePanel:737 */
```

- The tier channel is `var(--ride-card-tier)`, set inline from
  `tierStyle(tier).colorVar` — **never a hardcoded tier name or hue.** This is
  already the pattern at SagDock:397 and SagCandidatePanel:367.
- A card list uses `gap: var(--space-xs)` between cards. Never `6px`.
- De-emphasis is **`color`, not `opacity`** (see §5.4).

### 3.4 THE row recipe

*Derived from `NetControlPanel.svelte:4943` `.op-header` + the 44px floor in `.tab`
(`:4522`) and `--sag-hit` (app.css:218).*
*Applies to `.sb-row`, `.sup-row`, `.med-row`, `.sd-row`, `.cp-row-head`, `.rs-row`, `.sb-slot`, `.rpl-row`.*

```css
/* ride: list row */
display: flex;
align-items: center;
flex-wrap: wrap;
gap: var(--space-xs) var(--space-sm);    /* the ONLY gap pair a row uses */
min-height: 44px;                        /* every row, on every board */
padding: 0 var(--space-sm);              /* min-height owns the vertical */
font-size: var(--ride-t-body);
```

`min-height: 44px` on **every** board row is what stops the content jumping when
the operator switches SAG → Supply → Medical.

### 3.5 THE chip / badge recipe

*Derived from `NetControlPanel.svelte:4554` `.tab-count` (count) and `:4906`
`.priority-badge` (status pill).*

```css
/* ride: count chip — a number beside a label */
display: inline-flex;
align-items: center;
justify-content: center;
min-width: 18px;
height: 18px;                            /* fixed: a chip must never change row height */
padding: 0 var(--space-xs);
border-radius: var(--radius-full);
background: var(--color-primary);
color: var(--color-text-muted);
font-size: var(--ride-t-label);
font-weight: 600;
font-variant-numeric: tabular-nums;
line-height: 1;
```

```css
/* ride: status pill — a WORD describing state */
padding: var(--space-2xs) var(--space-sm);
border-radius: var(--radius-sm);
font-size: var(--ride-t-label);
font-weight: 700;
text-transform: uppercase;
letter-spacing: var(--ride-label-tracking);
white-space: nowrap;
flex-shrink: 0;
/* colour: --color-text-muted by default; a tier or semantic -text variant
   otherwise. NEVER a raw tier token on a word. */
```

### 3.6 THE button recipe — exactly three sizes

*Derived from `NetControlPanel.svelte:4415` `.cn-btn` (44), `:5745` `.btn-sm` (36),
`:5708` `.btn-mini` / `:5390` `.bar-btn` / `:5472` `.op-btn` (32).*

```css
/* ride: button — shared base */
display: inline-flex;
align-items: center;
justify-content: center;
gap: var(--space-xs);
border-radius: var(--radius-sm);
font-family: inherit;
font-weight: 600;
white-space: nowrap;
cursor: pointer;
transition: border-color var(--duration-fast), color var(--duration-fast),
            background var(--duration-fast);

/* --- size: primary / in-vehicle (the DEFAULT for ride mode) --- */
min-height: var(--sag-hit);              /* 44px */
padding: 0 var(--space-md);
font-size: var(--ride-t-body);

/* --- size: compact (desktop-only secondary actions) --- */
min-height: 36px;
padding: 0 var(--space-sm);
font-size: var(--ride-t-label);

/* --- size: mini (icon-only ✕, ⌄, numeric steppers) --- */
min-width: 32px; min-height: 32px;
padding: 0 var(--space-xs);
```

```css
/* variants */
.ride-btn--primary { background: var(--color-accent); border: none;
                     color: var(--color-on-accent); font-weight: 700; }
.ride-btn--default { background: var(--color-bg);
                     border: 1px solid var(--color-primary); color: var(--color-text); }
.ride-btn--quiet   { background: none;
                     border: 1px solid var(--color-primary); color: var(--color-text-muted); }
.ride-btn--danger  { background: none; border: 1px solid var(--color-error);
                     color: var(--color-error-text); }   /* -text variant, per app.css:63 */
.ride-btn:disabled { opacity: 0.45; cursor: not-allowed; }  /* one value, was 0.3/0.4/0.5 */
```

**The 44px size is the default in ride mode**, not the exception — §5.1.

---

## §4 The change list

Ordered by impact. Every entry is file · line · current · target · why.

### P0 — the vertical-jump fixes (these are the actual complaint)

| # | File:line | Current | Target | Why |
|---|---|---|---|---|
| 1 | SagDock:644 | `align-items: baseline` | `align-items: center` | `.cp-head-row` (SagCandidatePanel:661) is `center`; the two panels swap in the same slot and the title shifts. |
| 2 | SagDock:646 | `padding: var(--space-sm) var(--space-sm) var(--space-xs)` | `min-height: 44px; padding: 0 var(--space-sm)` | §3.2. Fixes the 8/4 asymmetry and pins the band height. |
| 3 | SagCandidatePanel:654 | `padding: var(--space-sm)` | `min-height: 44px; padding: 0 var(--space-sm)` on `.cp-head-row` only; move `.cp-head-row--sub` out of the band | §3.2. Makes `.cp-head` the same 44px as `.sd-head`, killing the 19px jump on dispatch focus. |
| 4 | SagCandidatePanel:669 | `.cp-head-row--sub { margin-top: 2px }` | `margin: 0; padding: var(--space-xs) var(--space-sm)` as a row below the band | Off-scale `2px`; also see #3. |
| 5 | SagDock:476 | `<div class="sd-item sd-group sd-group--toggle">` | keep the `div role="button"` (it must stay focusable) but give it the §3.1 recipe **and** `min-height` matching the `<h3>` groups | The toggle header sits 4px lower than the `<h3>` headers above it (`padding: 2px 0`, SagDock:729). |
| 6 | SagDock:484 | markup text `in motion` | `In motion` in markup + `text-transform: uppercase` in `.sd-group` | Same class as `⬒ WAITING` (SagDock:437), opposite casing. |
| 7 | SagDock:385, 437, 514 | markup text `⚑ NOT ON THE MAP`, `⬒ WAITING`, `🚐 NO POSITION` | `⚑ Not on the map`, `⬒ Waiting`, `🚐 No position` + `text-transform: uppercase` in CSS | Casing belongs in CSS (§3.1 rule 1); also makes the strings readable to screen readers as words. |
| 8 | SagCandidatePanel:369 | markup text `CANDIDATES for` | `Candidates for` + `text-transform: uppercase` on `.cp-kicker` | Same; currently the only mixed-case eyebrow in ride mode. |
| 9 | RideSituation:124 | `padding: 10px var(--space-md) 6px` | `min-height: 44px; padding: 0 var(--space-md)` | Two off-scale literals; §3.2. |
| 10 | SupplyBoard:263 / MedicalBoard:307 | `.sup-card` / `.med-card` have **no** `min-height` on `.sup-row` / `.med-row` | add `min-height: 44px` to `.sup-row` (:272) and `.med-row` (:316) | `.sb-row` is 44px (SagBoard:1108). Switching sub-tabs currently changes row height, so the whole board shifts. §3.4. |

### P1 — the section-header recipe (§3.1) applied

Each of these becomes exactly the §3.1 block.

| # | File:line | Class | Current deviations |
|---|---|---|---|
| 11 | SagBoard:1260 | `.sb-section-h` | `0.68rem` → `var(--ride-t-label)`; `0.04em` → `var(--ride-label-tracking)`; drop `margin: var(--space-xs) 0 0` (parent `.sb-detail` already has `gap: var(--space-sm)`, SagBoard:1183) |
| 12 | SagBoard:1199 | `.sb-detail-label` | `0.65rem` → `var(--ride-t-label)`; add `font-weight: 700`; `0.04em` → token |
| 13 | SagBoard:1413 | `.sb-inline-label` | `0.68rem` → `var(--ride-t-label)`; add `font-weight: 700`; add `letter-spacing: var(--ride-label-tracking)` |
| 14 | SagBoard:1031 | `.sb-attach-label` | `0.72rem` → `var(--ride-t-label)`; `0.04em` → token |
| 15 | SagDock:710 | `.sd-group` | `800` → `700`; `0.08em` → token; add `text-transform: uppercase`; drop `margin-top: var(--space-xs)` (`.sd-list` has `gap`, SagDock:704) |
| 16 | SagDock:729 | `.sd-group--toggle` | drop `padding: 2px 0`; inherit the §3.1 box from `.sd-group` |
| 17 | SagCandidatePanel:672 | `.cp-kicker` | `800` → `700`; `0.08em` → token; add `text-transform: uppercase` |
| 18 | RideSituation:127 | `.rs-heading` | `0.7rem` → `var(--ride-t-label)`; `0.06em` → token |
| 19 | RideSituation:207 | `.rs-closeout-heading` | `0.7rem` → `var(--ride-t-label)`; add `letter-spacing: var(--ride-label-tracking)`; `margin-bottom: 6px` → drop, add `gap: var(--space-xs)` to `.rs-closeout` (:202) |
| 20 | RideSituation:156 | `.rs-row-label` | `0.65rem` → `var(--ride-t-label)`; add tracking token |
| 21 | RideZone:105 | `.ride-zone-label` | `0.06em` → `var(--ride-label-tracking)` only. **Do not touch size or line-height** — see §6.5 |
| 22 | RideConfigSheet:244 | `.rcs-title` | `0.7rem` → `var(--ride-t-label)`; `0.03em` → token |
| 23 | SagLocationField:170 | `.slf-legend` | `0.7rem` → `var(--ride-t-label)`; `0.04em` → token; `padding: 0 4px` → `0 var(--space-xs)` |
| 24 | SagLocationField:192 | `.slf-label` | `0.68rem` → `var(--ride-t-label)`; add `font-weight: 700`, `text-transform: uppercase`, tracking token. **This is the single worst outlier**: `.slf-label` and `.src-label` sit in the *same dialog* (SagRequestComposer renders SagLocationField) and are styled differently — one uppercase bold, one lowercase regular. |
| 25 | SagRequestComposer:350 | `.src-label` | `0.7rem` → `var(--ride-t-label)`; `0.04em` → token |
| 26 | SagRequestComposer:410 | `.src-legend` | same as #25; `padding: 0 4px` → `0 var(--space-xs)` |
| 27 | SupplyComposer:353 | `.suc-label` | same as #25 |
| 28 | SupplyComposer:413 | `.suc-legend` | same as #26 |
| 29 | MedicalComposer:290 | `.mec-label` | same as #25 |
| 30 | SagDock:583 | `.sd-rail-title` | `9px` → `var(--ride-t-label)`; `800` → `700`; `0.08em` → token. 9px is below any legible floor on a sunlit tablet and is the smallest type in the app. |
| 31 | SagDock:605, 613, 620 | `.sd-rail-code 10px`, `.sd-rail-n 12px`, `.sd-rail-quiet/-grip 12px` | `var(--ride-t-label)` for the code/quiet/grip, `var(--ride-t-body)` for `.sd-rail-n` (it is a count the operator reads at a glance) |

### P2 — card / row / chip / button unification

| # | File:line | Current | Target | Why |
|---|---|---|---|---|
| 32 | SagBoard:1097 `.sb-card` | no tier channel | add `border-left: 3px solid var(--ride-card-tier, var(--color-primary))` + set it inline from `tierStyle(tier).colorVar` at SagBoard:538 | `.sd-card` (SagDock:739) and `.cp-head` (SagCandidatePanel:656) both carry it. The board — the surface where the operator spends the most time — is the only one without a tier channel at the card edge. |
| 33 | SagBoard:1094, SupplyBoard:260, MedicalBoard:304, SagDock:704, SagCandidatePanel:715 | list `gap: 6px` / `4px` | `gap: var(--space-xs)` everywhere | Five list containers, three gap values. |
| 34 | SagBoard:1107 `.sb-row` | `gap: 4px var(--space-sm)` | `gap: var(--space-xs) var(--space-sm)` | Same value, token form. |
| 35 | SagBoard:1109 `.sb-row` | `padding: 6px var(--space-sm)` | `padding: 0 var(--space-sm)` (min-height 44 already present) | Off-scale `6px`; §3.4. |
| 36 | RideSituation:146 `.rs-row` | `padding: 6px var(--space-md)` | `padding: 0 var(--space-md)` | Same. |
| 37 | SagDock:741 `.sd-card` | `padding: 6px var(--space-sm)`; `gap: 3px` | `padding: var(--space-sm)`; `gap: var(--space-xs)` | Two off-scale values; §3.3. |
| 38 | SagCandidatePanel:729/732 `.cp-row` | `padding: 6px var(--space-sm)`; `gap: 3px` | same as #37 | §3.3. |
| 39 | SagDock:778 `.sd-row`, SagCandidatePanel:662 `.cp-head-row`, :829 `.cp-cap`, :959 `.cp-check`, SagRequestComposer:369 `.src-tier-chip`, SupplyComposer:385 `.suc-tier-chip`, MedicalComposer:322 `.mec-chip` | `gap: 5px` | `gap: var(--space-xs)` | Seven occurrences of an off-scale `5px`. |
| 40 | SagCandidatePanel:762 `.cp-row-head`, SagBoard:897/914/958/1094/1317/1357/1401/1407/1422/1437, SupplyBoard:246/260/338, MedicalBoard:304/366/416/423, SagPanel:80, SagRequestComposer:363/408, SupplyComposer:379/424, MedicalComposer:316, RideSituation via RideConfigSheet:243/257 | `gap: 6px` | `gap: var(--space-xs)` (tight rows) or `var(--space-sm)` (action bars) | **24 occurrences of a value that is not on the scale.** This alone is most of the "messy" impression. |
| 41 | SagBoard:1438, :1367(`margin-top: 4px` in `.sup-actions`/`.med-actions` at SupplyBoard:339 / MedicalBoard:367), SagDock:858, SagCandidatePanel:669/919/952 | `margin-top: 2px` / `4px` | drop; let the parent `gap` own it | Margin + gap double-spacing is why sibling action rows sit at different heights. |
| 42 | SagBoard:916 | `.sb-vehicle { padding: 4px 8px }` | `padding: var(--space-xs) var(--space-sm)` | Token form. |
| 43 | SagBoard:944, :973 | `.sb-vehicle-out/.sb-vehicle-full/.sb-needs { padding: 2px 6px }` | `padding: var(--space-2xs) var(--space-sm)` + the §3.5 status-pill recipe | `6px` off-scale; also unifies three near-identical pills. |
| 44 | SagBoard:941 | `.sb-vehicle-out/-full { font-size: 0.64rem; letter-spacing: 0.06em }` | `var(--ride-t-label)`; `var(--ride-label-tracking)` | `0.64rem` appears exactly once in the codebase. |
| 45 | SagBoard:1222 | `.sb-tier-code { font-size: 0.625rem; font-weight: 800; letter-spacing: 0.06em }` | `var(--ride-t-label)`; `800`→`700`; tracking token | `0.625rem` appears exactly once. Compare `.sd-code` (SagDock:791) which uses no explicit size at all and `.ride-peek-code` (RidePeek:78) at `var(--ride-t-label)/800` — the same two-letter tier code, three treatments. |
| 46 | RidePeek:78 | `.ride-peek-code { font-weight: 800; letter-spacing: 0.06em }` | `700`; tracking token | See #45. |
| 47 | SagDock:791 | `.sd-code { font-weight: 800; letter-spacing: 0.04em }` | `700`; tracking token; add `font-size: var(--ride-t-label)` | See #45. |
| 48 | SagPanel:97–109 | `.sp-count { background: var(--color-bg); padding: 0 5px; font-size: 0.68rem; font-weight: 700 }` | `background: var(--color-primary); padding: 0 var(--space-xs); font-size: var(--ride-t-label); font-weight: 600; font-variant-numeric: tabular-nums; line-height: 1` | §3.5, matching `.tab-count` (NetControlPanel:4554). Also add the active-tab treatment `.sp-tab.active .sp-count { background: var(--color-accent); color: var(--color-on-accent) }` which NetControlPanel:4600 has and SagPanel does not. |
| 49 | SagBoard:1140 `.sb-status`, SupplyBoard:279 `.sup-status`, MedicalBoard:323 `.med-status`, SagBoard:1294 `.sb-slot-disp`, SagBoard:1372 `.sb-leg-status` | `0.7rem` / `0.68rem`, mixed `text-transform` | the §3.5 status-pill recipe, one size, all uppercase | Five status renderings in three files; `.sb-slot-disp` (:1294) is the only one that is *not* uppercase, so a slot's state reads as a different kind of thing from a request's state. |
| 50 | SagBoard:1465, SagCandidatePanel:909 | `700` vs `800` on `*-refusal-head` / `*-overcap-head` | both `700` | Same component, copied; see §2.5. |
| 51 | SagBoard:1469 | `.sb-refusal-head { letter-spacing: 0.03em }` | `var(--ride-label-tracking)` | `0.03em` appears twice in the codebase (here and RideConfigSheet:244). |
| 52 | SagRequestComposer:328, SupplyComposer:336, MedicalComposer:273, RidePendingList:164 | `.src/.suc/.mec/.rpl-title { font-size: 1.05rem; font-weight: 700 }` | `font-size: 0.95rem; font-weight: 600` | Match `.title` / `.cn-title` (NetControlPanel:4067, :4322). Four files, one edit each. |
| 53 | RideConfigSheet:240 | `.rcs-h2 { font-size: 0.9rem; font-weight: 700 }` | `font-size: 0.95rem; font-weight: 600` | Same; brings the fifth dialog title in line. |
| 54 | SagBoard:1134 `.sb-seq` `0.82rem`, SupplyBoard:276 `.sup-row` `0.8rem`, MedicalBoard:320 `.med-row` `0.8rem`, SagBoard:1280 `.sb-slot` `0.78rem`, SagBoard:1365 `.sb-leg-head` `0.78rem` | five sizes for list-row body text | `var(--ride-t-body)` (`0.875rem`) everywhere | `SAG 7` must be the same size on the board and the dock (§2.4). `0.875rem` is the already-tokenized ride body size and is more legible outdoors than `0.78rem`. |
| 55 | SagBoard:1160/1207/1229/1245, SupplyBoard:311/320/326/349/372, MedicalBoard:338/352/358/378/396/410, RideSituation:105/175/197, RidePendingList:213/230 | `0.72rem` / `0.75rem` / `0.78rem` secondary text | `var(--ride-t-label)` (`0.6875rem`) for metadata, `var(--ride-t-body)` for anything the operator reads aloud | Three near-identical sizes collapse to the two roles that already exist. |
| 56 | SagBoard:1078, SupplyBoard:253, MedicalBoard:297, RidePendingList:180 | empty-state `font-size: 0.82rem` | `var(--ride-t-body)` | `0.82rem` appears 6× and nowhere else in the app. |
| 57 | SagBoard:1252, :1014, SupplyBoard:355, MedicalBoard (`:disabled`), SagRequestComposer:459/511, SagDock:878, SagCandidatePanel:939 | `:disabled { opacity: 0.3 / 0.4 / 0.5 }` | `opacity: 0.45` (NetControlPanel:4432 `.cn-btn:disabled`) | Three disabled opacities across eight rules. |

### P3 — buttons onto the three-size ladder (§3.6)

| # | File:line | Current `min-height` | Target | Why |
|---|---|---|---|---|
| 58 | SagPanel:81 `.sp-tab` | `40px` | `44px` | `.tab` is 44 (NetControlPanel:4522); a sub-nav tab is a primary target. |
| 59 | SagBoard:879 `.sb-new`, SupplyBoard:228 `.sup-new`, MedicalBoard:280 `.med-new` | `40px` | `44px` (primary) | The "+ New …" button is the board's most-hit control, pressed one-handed in a moving vehicle. |
| 60 | SagBoard:1239 `.sb-action`, :1474 `.sb-dispatch-btn`, SupplyBoard:342 `.sup-btn`, MedicalBoard:371 `.med-btn`, RidePendingList:223 `.rpl-action` | `36px` | `44px` | These are the dispatch / load / deliver / release actions. See §5.1. |
| 61 | SagDock:862 `.sd-act`, SagCandidatePanel:922 `.cp-act` | `30px` (→ `40px` at ≤1199px, SagDock:909 / SagCandidatePanel:995) | `44px` at ≤1199px; `36px` above | The media query already acknowledges these are touched; 40 is 4px short of the bar it was written for. |
| 62 | SagCandidatePanel:624 `.cp-resort-btn` | **no `min-height`**, `padding: 2px 8px` → ≈20px tall | `min-height: 36px; padding: 0 var(--space-sm)` | Smallest interactive target in ride mode by a wide margin. |
| 63 | SagDock:662 `.sd-collapse` | `28px` | `32px` min + `44px` at ≤1199px | Mini-button floor (§3.6); it is the only way to shut the dock on a tablet. |
| 64 | SagBoard:1063 `.sb-vehicle-edit/-save/-cancel` | `28px` | `32px` | Mini floor. |
| 65 | SagBoard:1047 `.sb-vehicle-input` | `28px`, `width: 40px` | `min-height: 32px` | Seat/rack numeric stepper; below the mini floor. |
| 66 | SagBoard:1306 `.sb-slot-resolve` | `30px` | `36px` | Off-ladder value. |
| 67 | SagBoard:1004 `.sb-slot-bike`, :1322 `.sb-add-rider input`, :1348 `.sb-add-rider-btn`, SagLocationField:216 `.slf-coord-toggle`, SagRequestComposer:470 `.src-add-slot`, :456 `.src-slot-remove`, SupplyComposer:462 `.suc-add-item`, :141 `.suc-item-remove`, RideSituation:199 `.rs-link`, RideStrip:359 `.ride-ack` | `32px` | keep 32 for icon-only; raise text buttons to `36px`; **`.ride-ack` to `44px`** | `.ride-ack` is the EMERGENCY acknowledge — the single most consequential button in ride mode — and it is currently 32px. RideZone's 66px row contract (RideZone:77) has room for 44. |
| 68 | SagBoard:1427 `.sb-dest-select` | `34px` | `36px` | Off-ladder value that exists once. |
| 69 | SagCandidatePanel:960 `.cp-check` | `28px` | `44px` | A checkbox chosen with a thumb while deciding how many riders to send. |
| 70 | SagDock:863 `.sd-act { padding: 0 8px }`, SagCandidatePanel:924 `.cp-act { padding: 0 10px }` | `8px` vs `10px` | `0 var(--space-sm)` both | Same button, two paddings; `10px` is off-scale. |
| 71 | SagBoard:1064 `.sb-vehicle-edit… { padding: 0 8px }`, :1006 `.sb-slot-bike { padding: 0 4px }`, :1323 `.sb-add-rider input { padding: 0 8px }`, SagRequestComposer:402/415/425, SupplyComposer:418/429, SagLocationField:176 | literal `4px` / `8px` | `var(--space-xs)` / `var(--space-sm)` | Token form; 11 occurrences. |

### P4 — smaller, still worth doing

| # | File:line | Current | Target | Why |
|---|---|---|---|---|
| 72 | SagPanel:72 | `.sp-subnav { padding: var(--space-sm) var(--space-md) 0 }` | `padding: 0 var(--space-xs)` | `.tabs` (NetControlPanel:4507) has no top padding; the current 8px top makes the SAG sub-nav 8px taller than the parent Net Control tab strip directly above it, so two tab rows stack with different heights. |
| 73 | SagPanel:71 | `gap: var(--space-2xs)` | `gap: 2px` is correct but `.tabs` uses literal `2px`; keep the token | No change needed — noted so nobody "fixes" it the wrong way. |
| 74 | SagBoard:1179 | `.sb-detail { padding: var(--space-sm) var(--space-md) var(--space-md) }` | `padding: var(--space-sm)` | Currently the collapsed row's `SAG 7` starts at x=24 (`.sb` 16 + `.sb-row` 8) and the expanded `Riders`/`Legs` headers start at x=32 (`.sb` 16 + `.sb-detail` 16). Expanding a card **staggers the left edge by 8px**. |
| 75 | SagDock:676 | `.sd-notice/.sd-warn/.sd-muted { margin: var(--space-xs) var(--space-sm) }` vs SagCandidatePanel:694 `.cp-notice` same, but SagCandidatePanel:616 `.cp-resort { margin: 0 var(--space-sm) var(--space-xs) }` | one rule: `margin: 0 var(--space-sm)` + parent `gap` | Two stacked notices in `.cp` currently have different top gaps. |
| 76 | MedicalBoard:274 | `.med-toolbar { justify-content: flex-start }` and **no "show all" control** | add the `.med-showall` checkbox mirroring `.sup-showall` (SupplyBoard:243) / `.sb-showall` (SagBoard:894), and `justify-content: space-between` | Two of three boards can reveal terminal records; Medical cannot reach a cancelled/released notification at all from the UI. This is a **function gap**, not cosmetics — see §5.5. |
| 77 | SupplyBoard:379 `.sup-correction`, MedicalBoard:426 `.med-correction` | identical rules, two classes | one shared rule | Pure duplication; both are the same textarea. |
| 78 | SagBoard:1073–1089 | `.sb-no-vehicles, .sb-dispatch-empty, .sb-empty, .sb-no-legs` share a warning box, then `.sb-empty, .sb-no-legs` override it back to plain | split into `.ride-note--warn` and `.ride-note--muted` | The override-then-undo pattern is why the empty state reads as *almost* a warning. |
| 79 | RideConfigSheet:239 `.rcs-back` | no `min-height`, `font-size: 0.8rem` | `min-height: 44px`, `font-size: var(--ride-t-body)` | Back navigation with no touch target. |
| 80 | RideConfigSheet:262 | `.rcs-route-remove { width: 40px; height: 40px }` | `min-width: 44px; min-height: 44px` | §5.1. |
| 81 | RideSituation:212 | `margin-bottom: 6px` | `gap: var(--space-xs)` on `.rs-closeout` | Off-scale. |
| 82 | RideSituation:219 | `.rs-closeout-item { padding: 3px 0 }` | `min-height: 32px; padding: 0` | Off-scale `3px`; also a check-off list whose items are ~18px tall. |
| 83 | SagCandidatePanel:834 `.cp-bar { gap: 2px }`, :951 `.cp-chooser { gap: 2px; padding-top: 2px }` | literal `2px` | `var(--space-2xs)`; drop the `padding-top` | app.css:10–12 introduced `--space-2xs` precisely for this. |
| 84 | SagDock:885 | `.sd-act kbd { font-size: 0.9em }` vs SagCandidatePanel:943 `.cp-act kbd` (no size) | one rule, `font-size: 0.9em` on both | The same `<kbd>` hint renders at two sizes across the dock and the candidate panel. |
| 85 | SagDock:883 / SagCandidatePanel:944 | `kbd { margin-left: 4px }` | `var(--space-xs)` | Token form. |

---

## §5 Genuine usability problems, beyond the cosmetic

### 5.1 Touch targets — 19 controls below 44px on a surface used in a vehicle

`--sag-hit: 44px` already exists (app.css:218) and is used only for map markers.
Meanwhile the dispatch flow itself — `Dispatch` (SagBoard:816), `Confirm load`
(:730), `Confirm delivery` (:754), `Release` (:701) — is all `.sb-action` at
**36px** (SagBoard:1239). The candidate panel's `Send` is `.cp-act--primary` at
**30px** (SagCandidatePanel:922), rising only to 40px on tablet
(SagCandidatePanel:995).

The worst three:
- `.cp-resort-btn` (SagCandidatePanel:624) — **no `min-height` at all**, ≈20px.
- `.cp-check` (SagCandidatePanel:960) — 28px, and it is the "which riders go in
  this van" checkbox.
- `.ride-ack` (RideStrip:359) — 32px, and it is the EMERGENCY acknowledgement.

**Recommendation:** make 44px the ride-mode default (§3.6), reserve 36px for
desktop-only secondary actions, and 32px for icon-only glyph buttons. Use
`var(--sag-hit)` so the value has one home.

### 5.2 Information hierarchy — the board is flatter than the dock

`SagDock` gives every card a tier-coloured left border (SagDock:739) and the
candidate panel gives its header one (SagCandidatePanel:656). `SagBoard`'s
`.sb-card` (SagBoard:1097) has **no tier channel** — the tier survives only as a
16px glyph and a two-letter code buried mid-row (SagBoard:542–547). The board is
where an operator scans thirty requests; it is the one surface that most needs
the peripheral channel, and it is the one that lacks it. Change #32.

Related: `.sb-row` is a 12-element flex row (`SAG 7`, glyph, code, status, needs-
vehicle pill, vehicle, bibs, age, pickup→dropoff, chevron — SagBoard:540–560) with
every element at 0.68–0.82rem. Nothing in it is dominant. Compare `.rs-row`
(RideSituation:141) which gives the value line `0.95rem/700` against a `0.65rem`
label — a real hierarchy. The board row should promote `SAG 7` + the
needs-vehicle pill and demote the rest to `--ride-t-label`.

### 5.3 Contrast risks

Per app.css:63–74 the rule is: **tier/semantic colour on a word → `-text` variant.**
Good news first: **no raw tier token is used as a text colour anywhere in the ride
components.** The only raw-token uses are `border-color: var(--color-error)`
(SagBoard:1257, SupplyBoard:361, MedicalBoard:385) and
`border-top-color: var(--color-ride-emergency)` (RideStrip:265) — all borders,
all correct under the ≥3:1 rule.

The failures are **`opacity` used as de-emphasis on already-muted text**:

| File:line | Rule | Effective contrast on `--color-surface` |
|---|---|---|
| SagCandidatePanel:755 | `.cp-row--out { opacity: 0.55 }` over `--color-text-muted #aaa` | ≈ **2.4:1 — fails AA badly** |
| SagDock:758 | `.sd-card--motion { opacity: 0.75 }` | ≈ **4.0:1 — fails AA** |
| SagBoard:922 | `.sb-vehicle.released { opacity: 0.5 }` | ≈ **2.2:1 — fails AA** |
| SagDock:734 | `.sd-group-n { opacity: 0.8 }` on muted | ≈ 4.2:1 — borderline |
| SagBoard:1078 (`--color-warning` on `--color-ride-priority-soft`) | — | passes (app.css:107 documents amber at 7.40:1) |

**Recommendation:** replace `opacity` de-emphasis with an explicit colour. A
dimmed-but-legible state is `color: var(--color-text-muted)` plus a
`border-style: dotted` / `dashed` channel, which these components already use
(SagCandidatePanel:756, :751). `opacity` also dims the *border* that carries the
"excluded" meaning, which is the opposite of the intent stated at
SagCandidatePanel:753 ("Greyed, with a reason, never hidden").

Also: `.sd-rail-title` at **9px** (SagDock:584) and `.sd-rail-code` at **10px**
(SagDock:606) are below any usable size on a tablet in sunlight, regardless of
contrast ratio. Change #30/#31.

### 5.4 Mobile / breakpoints

- `SagDock` and `SagCandidatePanel` both carry `@media (max-width: 1199px)`
  (SagDock:903, SagCandidatePanel:989). **No other ride component has a
  breakpoint at all.** `SagBoard`, `SupplyBoard` and `MedicalBoard` — which are
  the bottom-sheet content on a phone — have no touch adaptation whatsoever.
  Their 36px buttons stay 36px on the phone.
- `SagBoard:1121` has a `@media (min-width: 1500px)` that re-orders `.sb-pickup`.
  1500px is not a breakpoint used anywhere else in the app (app.css uses 768/769
  and 480; the ride components use 1199/1200). It works, but it is a fourth,
  undocumented breakpoint — at minimum it should be a comment referencing the
  panel width it is actually about (`--panel-width: 480px`, app.css:48).
- `.sb-detail-grid` (SagBoard:1186) is `grid-template-columns: 1fr 1fr` with no
  breakpoint. Inside a 480px side panel minus padding that is two ~215px columns;
  inside the phone bottom sheet it is two ~160px columns holding strings like
  `Rest Stop Maxwell Chapel`. It should collapse to one column below ~400px.

### 5.5 Medical has no "show all" (functional gap)

`SagBoard:469` and `SupplyBoard:141` both offer a "Show complete/cancelled"
checkbox. `MedicalBoard:161` offers nothing, and `MedicalBoard:27` derives its
list from `$medicalOpen` only. A cancelled or released medical notification is
**unreachable from the UI** — which matters because those records are the ICS-214
trail. Change #76.

### 5.6 Two dispatch paths, two different UIs for the same decision

`SagBoard`'s inline dispatch (SagBoard:762–825) and `SagCandidatePanel`
(the whole file) both answer "which vehicle do I send". They share the API call
and the 409 handling — `SagCandidatePanel:15–17` says so explicitly — but the
board's version is a bare `<select>` (SagBoard:782) with no distance, no ETA and
no capacity pips, while the panel's is a ranked list. An operator who dispatches
from the board is making the decision with strictly less information than one who
dispatches from the map, and nothing on the board says so. Not a styling fix, but
it belongs in the record: the board's `Dispatch vehicle` button
(SagBoard:823) should arguably route to `startDispatchFocus()` rather than open a
second, weaker picker.

### 5.7 Keyboard hints are shown where there is no keyboard

`.sd-act kbd` (SagDock:419, 423, 427, 467, 533) and `.cp-act kbd`
(SagCandidatePanel:550) render `p` / `m` / `i` / `d` / `c` key hints
unconditionally — including inside the phone bottom sheet, where they are noise
occupying width on the app's narrowest surface. Suggest hiding them under the
existing `@media (max-width: 1199px)` blocks that already exist in both files.

---

## §6 Anti-recommendations — what NOT to change

These look inconsistent. They are deliberate. Each is documented in the source.

**6.1 `.sb-refusal` vs `.sb-overcap` must stay two different components, in two
different colours.** SagBoard:1441–1463 and SagCandidatePanel:189–193: *"A seat
refusal STATES A LIMIT; a rack overflow ASKS A QUESTION… only one of them has an
'anyway' button."* Red-bordered refusal vs amber-banded question. Do **not**
merge them, do not give them the same border treatment, and do not add an
override button to the refusal — the server refuses the retry, so the button
would be a lie. The only change §4 asks for is the `700`/`800` weight drift (#50).

**6.2 `.sd-group--flag` must stay un-muted.** SagDock:718–722: *"The only group
that is never muted: it is the one nothing else renders."* An unplaceable request
is invisible on every other surface. Keep `color: var(--color-warning)` and keep
`margin-top: 0` while the other groups get top spacing — it must sit at the very
top with no gap above it. Likewise keep `.sd-card--unplaced`'s dashed border and
`--color-ride-priority-soft` background (SagDock:752–755).

**6.3 The emergency treatment is deliberately louder and must stay so.**
`.rs-emergency` (RideSituation:84) uses a **4px** left border where cards use 3px
— do not normalize it to 3. `.ride-strip--emergency` (RideStrip:257) uses a 4px
top border and a pulse animation. `.sp-count-alert` (SagPanel:116) uses the
emergency-soft background where sibling counts are neutral. `.rs-ack` /
`.ride-ack` are the loudest buttons on their surfaces. All correct. (Raising
`.ride-ack` to 44px in #67 makes it *more* prominent, not less.)

**6.4 Tier colour must never be hardcoded.** app.css:139–143: the ladder is keyed
by **rank**, and the five tier names are agency data. Every spec in §3 uses
`var(--ride-card-tier, …)` / `var(--sd-tier-text, …)` set inline from
`tierStyle()` (`lib/rideMeta.ts:35`). Do not "simplify" `--sd-tier` /
`--cp-tier` away into named classes. Do not assume red = emergency in CSS.
Related: the two-letter code (`.sb-tier-code`, `.sd-code`, `.ride-peek-code`) is
the non-colour channel that separates rank 2 from rank 3 — they share amber
(app.css:145–146). **Never remove the code to tidy a row.**

**6.5 `RideZone`'s metrics are a height contract — do not touch them.**
RideZone:77–81: *"Row B is a 66px contract… these are the tightest metrics that
still clear 4.5:1."* Do not change `.ride-zone-hit`'s `padding: 2px var(--space-sm)`
(RideZone:89), `.ride-zone-label`'s `line-height: 1.15` (:107),
`.ride-zone-value`'s `1.05` (:121) or any font-size in that file. The only change
§4 asks for is swapping the `0.06em` literal for the tracking token (#21), which
is a no-op at the same value. Same for `RideStrip`'s `grid-template-rows`
(RideStrip:239, 249, 253) — three separate regressions are documented there.

**6.6 `flex-shrink: 0` on `.sd-card` (SagDock:749) and `.cp-row`
(SagCandidatePanel:737) is load-bearing.** Both carry comments describing the
exact bug removing it caused: Send buttons rendering on top of the row beneath
them inside the phone sheet. Keep it in the §3.3 card recipe.

**6.7 `.sb-pickup`'s wrap behaviour (SagBoard:1113–1127) is deliberate.**
*"The where-to-where must never be the thing that gets ellipsed away."* Keep
`flex: 1 1 100%` / `order: 9` below 1500px. The only suggestion is to document
what 1500px means (§5.4).

**6.8 Count badges must stay visually distinct from status pills.** `.sp-count`
(SagPanel:97) is a pill-shaped number; `.sb-needs` (SagBoard:968) is a
rectangular word-pill. They are different information (how many vs what state)
and §3.5 keeps them as two recipes on purpose. Do not collapse them into one.

**6.9 Do not promote `--ride-t-*` into a global type scale as part of this work.**
app.css:178–182 explicitly scopes them: *"Deliberately ride-prefixed, not a global
scale — promoting them app-wide is later cleanup work."* This pass uses them
inside ride mode only.

**6.10 `.cp-row--stale` (dashed) / `.cp-row--partial` (amber left border) /
`.cp-row--out` (dotted) are three distinct non-colour channels** and must remain
distinguishable from each other by border-style alone. The only change is
replacing the `opacity` on `--out` with an explicit colour (§5.3), which
*preserves* the dotted channel that opacity currently dims away.

**6.11 The composers are the one consistent family — do not "vary" them.**
`SagRequestComposer`, `SupplyComposer` and `MedicalComposer` are byte-for-byte
identical in their title/label/legend/chip/button recipes. That consistency is an
asset. The §4 changes (#25–#29, #52) apply the *same* edit to all three so they
stay identical. Never fix one composer without the other two.
