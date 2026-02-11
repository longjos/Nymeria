# Nymeria Logo Design Brief

## The Project

**Nymeria** is an open-source APRS client for amateur radio operators. APRS (Automatic Packet Reporting System) is a real-time tactical communication protocol used by ham radio operators — particularly during emergency communications (hurricanes, wildfires, search and rescue).

Nymeria is a modern, web-based client that runs in the browser. It tracks stations on a live map, handles messaging, manages emergency nets, and supports both internet and RF (radio frequency) connections. It's built for individuals and teams — multiple operators can share a single station.

The name comes from the direwolf in *Game of Thrones* — Arya Stark's wolf. Fierce, loyal, independent.

---

## What We Need

A **logomark** (icon) and **wordmark** (icon + "Nymeria" text) that:

1. Works as a **favicon** (16x16, 32x32) — must be recognizable at tiny sizes
2. Works as a **single-color SVG** — no gradients, no multi-color
3. Incorporates a **wolf** — the viewer should recognize "wolf" without needing explanation
4. Feels **modern and confident** — not dated, not corporate, not generic
5. Deliverables: SVG source files, PNG exports at standard sizes (16, 32, 64, 128, 256, 512)

---

## Brand Personality

| Trait | Expression |
|---|---|
| **Reliable** | This runs during emergencies. It needs to feel trustworthy, not playful. |
| **Modern** | The APRS ecosystem is full of software that looks like it was built in 2003. Nymeria is the modern alternative. |
| **Technical** | The audience is amateur radio operators — smart, hands-on, engineering-minded. Don't dumb it down. |
| **Community** | Ham radio is collaborative. Nymeria is built for teams. The tone is warm, not cold. |
| **Open Source** | Approachable, not corporate. Made by hams, for hams. |

**If Nymeria were a person:** A calm, experienced search-and-rescue volunteer who shows up with better gear than everyone else but never brags about it.

---

## The Wolf

The wolf is the heart of the mark. Some direction:

- **Alert, not aggressive.** This isn't a sports team mascot snarling at you. Think: a wolf watching the horizon. Attentive. Ready.
- **A howling wolf is fair game** — and there's a natural metaphor here (howling = broadcasting a radio signal). If you go this direction, consider incorporating subtle radio wave arcs emanating from the howl. But only if it feels natural, not forced.
- **Profile vs. front-facing:** Either works. A side profile with the ear, snout, and jaw clearly defined tends to read better at small sizes. A front-facing wolf with two prominent ears can work as a more symmetric mark.
- **The ear is the key feature.** At favicon size, the pointed wolf ear is what separates "wolf" from "generic blob." Exaggerate it.
- **Single color means the silhouette does all the work.** No relying on internal detail for recognition. The outline shape alone must say "wolf."

---

## What to Avoid

These are the cliches we've seen and don't want:

- **Angular geometric polygon wolves.** Stock logo sites are drowning in these. Low-poly, faceted wolf heads made of triangles. Played out.
- **Wolf in a shield/circle/hexagon.** Container shapes feel corporate and dated.
- **Perfect bilateral symmetry.** Reads as computer-generated. A touch of asymmetry feels intentional and designed.
- **Overly detailed illustrations.** Won't survive favicon reduction. Needs to be bold and simple.
- **Howling at a moon.** Unless it's extremely well executed and fresh, this is clip art territory.
- **The "tech company wolf."** Sleek, chrome, abstract. We're amateur radio, not a fintech startup.
- **Wolves with circuit board patterns, antenna ears, or radio towers composited into the body.** Heavy-handed metaphor mixing. If the radio connection is there, it should be subtle (like arcs near a howling mouth, not an antenna growing out of the head).

---

## Visual References & Mood

**Things that feel right:**
- The confidence of the WWF panda — a bold, simple silhouette that's instantly recognizable
- The warmth of the Firefox logo — stylized, friendly, but not childish
- Woodcut / linocut aesthetics — bold shapes, visible craft, not algorithmically perfect
- Japanese mon (family crests) — extreme simplification while retaining identity
- National park logos — warm, trustworthy, made for the outdoors

**Things that feel wrong:**
- Dribbble geometric polygon wolves
- Gaming/esports team mascots (too aggressive)
- Minimalist tech logos that could be anything (too abstract)
- Stock illustration style (too generic)

---

## Color

The primary brand color is **#e94560** (a warm, confident red-pink). The logo should be designed in black first, then applied in brand color. It must work:

- Red (#e94560) on dark background (#111118 / #1a1a2e)
- Red (#e94560) on white background
- White on dark background
- Black/dark on white background

---

## Typography (Wordmark)

The wordmark pairs the icon with "Nymeria" set in **Inter** (weight 800, letter-spacing -0.02em). The designer does not need to create custom lettering — just ensure the icon and Inter 800 feel balanced together at various sizes.

---

## Usage Context

Where this logo will appear:

- **Browser favicon** (16x16, 32x32) — the most demanding use case
- **GitHub repository** social preview and README header
- **GitHub Pages** marketing site (dark background, hero section)
- **In-app** — top-left corner of the web UI, alongside the wordmark
- **Documentation** headers
- **Potential:** stickers, QR code cards for ham radio events

---

## Technical Requirements

- **Format:** SVG (vector source), PNG exports
- **Single path preferred** for the icon (or minimal paths). Simple SVG structure that's easy to embed inline.
- **ViewBox:** 0 0 [square] — the mark should work in a square bounding box
- **No embedded fonts, no raster images, no gradients, no effects**
- **Color applied via `fill` attribute** so it can be easily themed via CSS (`fill="currentColor"`)

---

## Summary

We need a wolf that's bold enough to read at 16 pixels, confident enough to anchor a modern web app, warm enough to represent a community of volunteers, and distinctive enough that nobody confuses it with a stock logo. The wolf should feel like it was designed by someone who cares — not generated by an algorithm.

Simple. Bold. Wolf. That's the brief.
