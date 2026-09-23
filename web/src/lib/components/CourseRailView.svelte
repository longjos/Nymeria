<script lang="ts">
	/**
	 * The course rail (docs/course-rail-spec.md), drawn from a RailModel. It
	 * answers one question — "where is it ALONG THE COURSE, in the miles we
	 * speak on the air?" — and the map answers every other one.
	 *
	 * Presentational only: the model (railModel.buildRailModel) holds every
	 * decision, so the three rails — the ride strip, the SituationBoard panel
	 * and the agency dashboard, which keeps its own data — show one course one
	 * way. This component only lays it out in pixels and draws it.
	 *
	 * Lanes, top to bottom: [WX brackets, panel only] · LEAD · COURSE · SWEEP ·
	 * [stop names, panel only] · READOUT. At strip density the four core lanes
	 * are exactly the strip's 60px row. Every x on every lane goes through ONE
	 * axis on ONE track element (B4).
	 *
	 * LEAD and SWEEP are REPORTED positions only. The user's rule: "We'd always
	 * take the reported sweep over the GPS." A sweep van's live fix is an
	 * ordinary roster pip and nothing more.
	 */
	import { pointAtChainage } from '$lib/routeDistance';
	import { STOP_GLYPHS, ageState, ageText } from '$lib/rideMeta';
	import { stationCategoryMeta } from '$lib/stationCategoryMeta';
	import { dodgeStops, clusterPips, type PipCluster } from '$lib/courseRail';
	import type { RailModel, RailStopModel } from '$lib/railModel';
	import type { RosterOnCourse } from '$lib/rosterCourse';
	import type { StationCategory } from '$lib/types';
	import RideTierGlyph from './RideTierGlyph.svelte';
	import WxTierGlyph from './WxTierGlyph.svelte';

	let {
		model,
		now,
		density = 'strip',
		presentation = false,
		onStopActivate,
		onRosterActivate,
		onFlyTo,
		onSweepPassed,
		onRosterList
	}: {
		model: RailModel;
		/** ms epoch; ages are computed against it. */
		now: number;
		/** 'strip': the ride strip's fixed 60px row. 'panel': taller, with stop
		 *  names and weather brackets — SituationBoard and the dashboard. */
		density?: 'strip' | 'panel';
		/** The dashboard's large-screen mode: bigger type, same layout. */
		presentation?: boolean;
		onStopActivate?: (annotationId: string) => void;
		/** A roster member was chosen. Without it, pips are still listed. */
		onRosterActivate?: (checkInId: string) => void;
		/** Fly the map to a point — LEAD, SWEEP, a gate or an incident. */
		onFlyTo?: (lat: number, lon: number) => void;
		onSweepPassed?: () => void;
		/** Open the roster — for the "not near the course" count. */
		onRosterList?: () => void;
	} = $props();

	const MI = 1609.344;
	let panel = $derived(density === 'panel');
	/** Minimum centre spacing before stops are spread apart (spec §B3). */
	let STOP_MIN_PX = $derived(panel ? 22 : 18);
	/** Roster pips closer than this become one count badge (spec R13). */
	const PIP_MERGE_PX = 10;
	/** A pip never sits on a stop glyph: pushed this far from a stop centre. */
	let STOP_CLEAR_PX = $derived(panel ? 11 : 9);
	/** One tap, never a precision tap: a 44px window on the course lane (R16). */
	const TOUCH_HALF_PX = 22;
	/** A stop's name is printed under it only when it has this much room. */
	const NAME_ROOM_PX = 76;
	const CATEGORY_ORDER: StationCategory[] = ['medical', 'sag', 'marshal', 'mobile', 'tactical', 'command', 'fixed', 'general'];

	let trackW = $state(0);

	let axis = $derived(model.axis);
	let edges = $derived(model.edges);
	let phase = $derived(model.phase);

	const px = (pct: number | null) => (pct == null ? null : (pct / 100) * trackW);
	const pctOfMile = (mile: number | null | undefined) => (mile == null ? null : axis.pct(mile * MI));

	/** Chainage -> the map. pointAtChainage is the EXACT direction of the
	 *  projection (mile -> point), so this never lands on the wrong leg. */
	function flyToChainage(m: number | null | undefined) {
		const idx = model.routeIndex;
		if (m == null || !idx || !onFlyTo) return;
		const p = pointAtChainage(idx, m);
		if (p) onFlyTo(p.lat, p.lon);
	}
	const flyToMile = (mile: number | null | undefined) => flyToChainage(mile == null ? null : mile * MI);
	function flyToEdge(e: typeof edges.lead) {
		if (!e) return;
		if (e.mile != null) flyToMile(e.mile);
		else if (e.checkpointId) onStopActivate?.(e.checkpointId);
	}
	const enterActivates = (fn: () => void) => (e: KeyboardEvent) => {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			fn();
		}
	};

	// ---- stops ----

	interface StopView extends RailStopModel {
		truePx: number;
		drawPx: number;
		dodged: boolean;
		showName: boolean;
		/** The name as printed: shortName, or the label minus a prefix every
		 *  stop shares ("Rest Stop Maxwell Chapel" -> "Maxwell Chapel"). */
		name: string;
		nameMaxPx: number;
		nameAlign: 'start' | 'middle' | 'end';
	}

	/** A leading run of words EVERY named stop shares, e.g. "Rest Stop ". It is
	 *  the least informative part of each name and the first to be cut, so a
	 *  truncated "Rest Stop M…" hid exactly what told the stops apart. */
	function sharedPrefix(labels: string[]): string {
		if (labels.length < 2) return '';
		const words = labels.map((l) => l.trim().split(/\s+/));
		const out: string[] = [];
		for (let i = 0; ; i++) {
			const w = words[0][i];
			if (w == null || !words.every((ws) => ws[i] === w && ws.length > i + 1)) break;
			out.push(w);
		}
		return out.length ? out.join(' ') + ' ' : '';
	}

	let stops = $derived.by((): StopView[] => {
		if (trackW <= 0) return [];
		const raw: { id: string; px: number }[] = [];
		for (const s of model.stops) {
			const p = px(s.pct);
			if (p != null) raw.push({ id: s.id, px: p });
		}
		const dodged = new Map(dodgeStops(raw, trackW, STOP_MIN_PX).map((d) => [d.id, d]));
		const out: StopView[] = [];
		for (const s of model.stops) {
			const d = dodged.get(s.id);
			if (d) out.push({ ...s, truePx: d.truePx, drawPx: d.drawPx, dodged: d.dodged, showName: false, name: s.label, nameMaxPx: 0, nameAlign: 'middle' });
		}
		out.sort((a, b) => a.drawPx - b.drawPx);
		// A name is printed only where it has room; otherwise it is in the
		// title and the accessible name, never overprinted (B10's panel half).
		// Among stops that share a leading phrase, only those that share it
		// lose it — the finish keeps "Finish".
		// The largest group of named stops sharing a first word ("Rest …"), so a
		// Start or Finish anywhere in the list cannot stop it being found.
		const groups = new Map<string, string[]>();
		for (const st of out) {
			if (st.shortName) continue;
			const w = st.label.trim().split(/\s+/)[0] ?? '';
			groups.set(w, [...(groups.get(w) ?? []), st.label]);
		}
		const biggest = Array.from(groups.values()).sort((x, y) => y.length - x.length)[0] ?? [];
		const prefix = sharedPrefix(biggest);
		out.forEach((s, i) => {
			const left = i > 0 ? s.drawPx - out[i - 1].drawPx : Infinity;
			const right = i < out.length - 1 ? out[i + 1].drawPx - s.drawPx : Infinity;
			// Each gap is split evenly between the two names that border it, so
			// no two names can meet. A centred name takes the smaller half on
			// both sides; an end name anchors inward and takes its one half.
			const halfL = left / 2 - 4;
			const halfR = right / 2 - 4;
			const nameAlign: StopView['nameAlign'] = s.drawPx < 50 ? 'start' : s.drawPx > trackW - 50 ? 'end' : 'middle';
			const room = nameAlign === 'start' ? halfR + 6 : nameAlign === 'end' ? halfL + 6 : 2 * Math.min(halfL, halfR);
			s.nameMaxPx = Math.min(160, room);
			s.nameAlign = nameAlign;
			s.name = (s.shortName || (prefix && s.label.startsWith(prefix) ? s.label.slice(prefix.length) : s.label)).replace(/\s+/g, ' ');
			s.showName = panel && s.nameMaxPx >= NAME_ROOM_PX / 2;
		});
		return out;
	});

	let wx = $derived(
		panel
			? model.wx
					.map((w) => ({ w, from: px(w.fromPct), to: px(w.toPct) }))
					.filter((b): b is { w: typeof b.w; from: number; to: number } => b.from != null && b.to != null)
			: []
	);

	// ---- lead / sweep ----

	let leadPx = $derived(px(model.leadPct));
	let sweepPx = $derived(px(model.sweepPct));
	let leadAge = $derived(edges.lead ? ageState(edges.lead.at, now) : 'never');
	let sweepAge = $derived(edges.sweep ? ageState(edges.sweep.at, now) : 'never');
	let leadAgeText = $derived(edges.lead ? ageText(edges.lead.at, now) : '');
	let sweepAgeText = $derived(edges.sweep ? ageText(edges.sweep.at, now) : '');

	/** Which side of its chevron a label goes. The chevron is always exactly
	 *  AT the marker's x; the label sits beside it — right by default, left
	 *  near the right end so it never runs off the track. One row, because the
	 *  edge lanes are 12px and a stacked label spilled into the readout. */
	function anchor(x: number | null, labelChars = 10, avoid: number[] = []): 'after' | 'before' | 'bare' {
		if (x == null) return 'after';
		// ~7px per 11px-bold character, plus the chevron and gap.
		const w = labelChars * 7 + 14;
		const hits = (from: number, to: number) => avoid.some((a) => a >= from - 6 && a <= to + 6);
		const afterOk = x + w <= trackW && !hits(x, x + w);
		const beforeOk = x - w >= 0 && !hits(x - w, x);
		// Yield to an incident glyph in the same lane (spec §B3).
		if (afterOk) return 'after';
		if (beforeOk) return 'before';
		// Neither side is clear (a narrow panel): drop the text, keep the
		// chevron. The position is still in the readout, the title and the
		// accessible name — only the overprint is lost.
		return 'bare';
	}

	// ---- field band: swept / riders on course / not yet reached ----

	let band = $derived.by(() => {
		if (phase === 'pre-start' || trackW <= 0) return null; // whole line dashed
		if (edges.inverted) return null; // order? — neutral line, readout says why
		const s = sweepPx ?? 0; // no sweep yet: riders stretch back to the start
		const l = leadPx;
		if (l == null) return sweepPx == null ? null : { swept: s, fieldEnd: null as number | null };
		return { swept: s, fieldEnd: Math.max(s, l) };
	});

	// ---- roster ----

	function rank(r: RosterOnCourse): number {
		const c = CATEGORY_ORDER.indexOf(r.category as StationCategory);
		const a = r.age === 'fresh' ? 0 : r.age === 'aging' ? 1 : 2;
		return (c < 0 ? CATEGORY_ORDER.length : c) * 10 + a;
	}

	let rosterById = $derived(new Map(model.roster.map((r) => [r.checkInId, r])));

	let clusters = $derived.by((): PipCluster[] => {
		if (trackW <= 0) return [];
		const pips: { id: string; px: number; rank: number }[] = [];
		for (const r of model.roster) {
			if (r.state === 'on-course' && r.chainageMeters != null) {
				const p = px(axis.pct(r.chainageMeters));
				if (p != null) pips.push({ id: r.checkInId, px: p, rank: rank(r) });
			} else if (r.state === 'ambiguous' && r.candidatesMiles) {
				// Never one mile: a ghost at each possibility, counted once.
				r.candidatesMiles.forEach((m, i) => {
					const p = px(pctOfMile(m));
					if (p != null) pips.push({ id: `${r.checkInId}#${i}`, px: p, rank: rank(r) });
				});
			}
		}
		return clusterPips(pips, PIP_MERGE_PX, stops.map((s) => s.drawPx), STOP_CLEAR_PX);
	});

	const baseId = (pipId: string) => pipId.split('#')[0];
	const membersOf = (c: PipCluster) => Array.from(new Set(c.members.map(baseId)));

	function pipClass(r: RosterOnCourse): string {
		if (r.state === 'ambiguous') return 'sr-pip--ghost';
		if (r.handPlaced) return 'sr-pip--hand';
		return `sr-pip--${r.age === 'never' ? 'stale' : r.age}`;
	}

	function catColor(category: string): string {
		return stationCategoryMeta[category as StationCategory]?.color ?? stationCategoryMeta.general.color;
	}

	function whereText(r: RosterOnCourse): string {
		if (r.state === 'ambiguous' && r.candidatesMiles) {
			return `mile ${r.candidatesMiles.map((m) => m.toFixed(1)).join(' or ')} — shared road, can't tell which`;
		}
		return r.chainageMeters != null ? `mile ${(r.chainageMeters / MI).toFixed(1)}` : '';
	}

	function memberText(r: RosterOnCourse): string {
		const cat = stationCategoryMeta[r.category as StationCategory]?.label ?? r.category;
		const name = r.tacticalCall ? `${r.tacticalCall} (${r.callsign})` : r.callsign;
		const age = r.handPlaced ? 'position set by hand' : r.age === 'stale' ? `stale, ${ageText(r.positionAt, now)} old` : `${ageText(r.positionAt, now)} old`;
		// R0.4: the sweep unit's GPS and the reported sweep are shown side by
		// side here and ONLY here. They disagree all the time — reports lag —
		// so this is information, never a warning.
		const sweepNote =
			r.isSweepUnit && edges.sweep?.mile != null
				? ` · sweep unit · last sweep report mi ${edges.sweep.mile.toFixed(1)} (${ageText(edges.sweep.at, now)})`
				: r.isSweepUnit
					? ' · sweep unit'
					: '';
		return `${name} · ${cat} · ${whereText(r)} · ${age}${sweepNote}`;
	}

	// ---- shutoffs and incidents: drawn only where they really are ----

	let gates = $derived(
		model.gates.map((s) => ({ s, x: px(s.pct) })).filter((g): g is { s: typeof g.s; x: number } => g.x != null)
	);
	let incidents = $derived(
		model.incidents.map((p) => ({ p, x: px(p.pct) })).filter((i): i is { p: typeof i.p; x: number } => i.x != null)
	);

	// ---- readout ----

	const edgeRead = (e: typeof edges.lead, label: string, ageTxt: string, st: string) => {
		if (!e) return `${label} not reported`;
		const where = e.mile != null ? `mi ${e.mile.toFixed(1)}` : e.seq != null ? `stop ${e.seq}` : '—';
		return `${label} ${where} · ${st === 'stale' ? `stale ${ageTxt}` : ageTxt}`;
	};
	let leadRead = $derived(edgeRead(edges.lead, model.leadLabel, leadAgeText, leadAge));
	let sweepRead = $derived(edgeRead(edges.sweep, model.sweepLabel, sweepAgeText, sweepAge));
	let spreadRead = $derived(
		edges.inverted ? 'order?' : edges.spreadMiles != null ? `spread ${edges.spreadMiles.toFixed(1)} mi` : ''
	);
	let scaleRead = $derived(
		axis.mode === 'mile' && axis.totalMiles != null ? `course ${axis.totalMiles.toFixed(1)} mi` : 'stops evenly spaced — not to scale'
	);
	let notNear = $derived(model.unplaced.offCourse);
	let placedCount = $derived(model.roster.filter((r) => r.state === 'on-course' || r.state === 'ambiguous').length);
	let atStopCount = $derived(model.roster.filter((r) => r.state === 'at-stop').length);
	/** The spread's time, as what it actually is (spec §A3.6). */
	let spreadTitle = $derived(
		edges.inverted
			? 'Sweep is reported ahead of lead — check the last passages'
			: edges.spreadMiles != null && model.sweepSpeedMph && model.sweepSpeedMph > 0
				? `At its reported ${model.sweepSpeedMph.toFixed(0)} mph, sweep is about ${Math.round((edges.spreadMiles / model.sweepSpeedMph) * 60)} min behind lead's position`
				: 'Miles between lead and sweep'
	);

	/** Spoken summary for the rail group (spec §B5) — not a live region. */
	let summary = $derived(
		[
			edges.lead ? `Lead ${edges.lead.mile != null ? `mile ${edges.lead.mile.toFixed(1)}` : `stop ${edges.lead.seq}`}, ${leadAgeText.replace('m', ' minutes')} ago.` : 'Lead not reported.',
			edges.sweep ? `Sweep ${edges.sweep.mile != null ? `mile ${edges.sweep.mile.toFixed(1)}` : `stop ${edges.sweep.seq}`}, ${sweepAgeText.replace('m', ' minutes')} ago.` : 'Sweep not reported.',
			edges.inverted ? 'Sweep is reported ahead of lead — check the last passages.' : edges.spreadMiles != null ? `${edges.spreadMiles.toFixed(1)} miles between them.` : '',
			`${placedCount} roster member${placedCount === 1 ? '' : 's'} on the course, ${atStopCount} at stops, ${notNear} not near the course.`
		]
			.filter(Boolean)
			.join(' ')
	);

	// ---- keyboard: one tab stop, arrows walk everything in mile order ----

	type Item =
		| { kind: 'stop'; key: string; x: number; stop: StopView }
		| { kind: 'lead' | 'sweep'; key: string; x: number }
		| { kind: 'cluster'; key: string; x: number; cluster: PipCluster }
		| { kind: 'gate'; key: string; x: number; name: string }
		| { kind: 'incident'; key: string; x: number; label: string };

	let items = $derived.by((): Item[] => {
		const list: Item[] = [];
		for (const s of stops) list.push({ kind: 'stop', key: `s:${s.id}`, x: s.drawPx, stop: s });
		if (leadPx != null) list.push({ kind: 'lead', key: 'lead', x: leadPx });
		if (sweepPx != null) list.push({ kind: 'sweep', key: 'sweep', x: sweepPx });
		clusters.forEach((c, i) => list.push({ kind: 'cluster', key: `c:${i}:${c.members.join(',')}`, x: c.px, cluster: c }));
		for (const g of gates) list.push({ kind: 'gate', key: `g:${g.s.id}`, x: g.x, name: g.s.name });
		for (const i of incidents) list.push({ kind: 'incident', key: `i:${i.p.id}`, x: i.x, label: i.p.label });
		return list.sort((a, b) => a.x - b.x);
	});

	let focusKey = $state<string | null>(null);
	let refs = $state<Record<string, HTMLElement | undefined>>({});
	let tabKey = $derived(items.some((i) => i.key === focusKey) ? focusKey : (items[0]?.key ?? null));

	function focusAt(i: number) {
		const it = items[Math.max(0, Math.min(items.length - 1, i))];
		if (!it) return;
		focusKey = it.key;
		refs[it.key]?.focus();
	}

	function onRailKey(e: KeyboardEvent) {
		const cur = items.findIndex((i) => i.key === tabKey);
		if (e.key === 'ArrowRight') { focusAt(cur + 1); e.preventDefault(); }
		else if (e.key === 'ArrowLeft') { focusAt(cur - 1); e.preventDefault(); }
		else if (e.key === 'Home') { focusAt(0); e.preventDefault(); }
		else if (e.key === 'End') { focusAt(items.length - 1); e.preventDefault(); }
		else if (e.key === 'Escape' && popover) { closePopover(); e.preventDefault(); }
	}

	// ---- popovers (cluster detail, touch window, legend) — in the top layer,
	//      because the strip clips its overflow ----

	let popover = $state<null | { kind: 'members'; ids: string[]; x: number; y: number; returnKey: string | null } | { kind: 'legend'; x: number; y: number }>(null);

	function topLayer(node: HTMLElement) {
		if (typeof node.showPopover !== 'function') return;
		try {
			node.setAttribute('popover', 'manual');
			node.showPopover();
		} catch {
			node.removeAttribute('popover');
		}
		return {
			destroy() {
				try {
					if (node.isConnected) node.hidePopover();
				} catch {
					/* already closed */
				}
			}
		};
	}

	function openMembers(ids: string[], anchorEl: Element, returnKey: string | null) {
		const r = anchorEl.getBoundingClientRect();
		popover = { kind: 'members', ids, x: r.left + r.width / 2, y: r.top, returnKey };
	}

	function closePopover() {
		const back = popover && popover.kind === 'members' ? popover.returnKey : null;
		popover = null;
		if (back) refs[back]?.focus();
	}

	function activateCluster(c: PipCluster, el: Element, key: string) {
		const ids = membersOf(c);
		if (ids.length === 1 && onRosterActivate) onRosterActivate(ids[0]);
		else openMembers(ids, el, key);
	}

	/** R16: a tap anywhere on the course lane opens everything within 22px. */
	function onCourseLaneClick(e: MouseEvent) {
		if ((e.target as HTMLElement).closest('.sr-stop, .sr-pip, .sr-badge, .sr-gate')) return;
		const lane = e.currentTarget as HTMLElement;
		const x = e.clientX - lane.getBoundingClientRect().left;
		const near = clusters.filter((c) => Math.abs(c.px - x) <= TOUCH_HALF_PX);
		const ids = Array.from(new Set(near.flatMap(membersOf)));
		if (ids.length === 1 && onRosterActivate) onRosterActivate(ids[0]);
		else if (ids.length > 0) {
			popover = { kind: 'members', ids, x: e.clientX, y: lane.getBoundingClientRect().top, returnKey: null };
		}
	}

	// First-run legend, once per device — convenience only (spec §B4.3).
	const LEGEND_SEEN_KEY = 'nymeria:courseRailLegendSeen';
	let legendBtn = $state<HTMLElement | undefined>();
	function openLegend() {
		const r = legendBtn?.getBoundingClientRect();
		if (!r) return;
		popover = { kind: 'legend', x: r.right, y: r.top };
	}
	$effect(() => {
		if (!legendBtn || trackW <= 0) return;
		let seen = true;
		try {
			seen = localStorage.getItem(LEGEND_SEEN_KEY) === '1';
			if (!seen) localStorage.setItem(LEGEND_SEEN_KEY, '1');
		} catch {
			seen = true; // no storage: never nag
		}
		if (!seen) openLegend();
	});

	$effect(() => {
		if (!popover) return;
		const onDoc = (e: PointerEvent) => {
			if (!(e.target as HTMLElement).closest('.sr-popover, .sr-badge, .sr-legend-btn, .sr-course')) closePopover();
		};
		const onKey = (e: KeyboardEvent) => {
			if (e.key === 'Escape') closePopover();
		};
		document.addEventListener('pointerdown', onDoc);
		document.addEventListener('keydown', onKey);
		return () => {
			document.removeEventListener('pointerdown', onDoc);
			document.removeEventListener('keydown', onKey);
		};
	});

	const summaryId = `sr-summary-${Math.random().toString(36).slice(2, 8)}`;
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="sr" class:sr--panel={panel} class:sr--presentation={presentation} role="group" aria-label="Course rail" aria-describedby={summaryId} onkeydown={onRailKey}>
	<p id={summaryId} class="sr-only">{summary}</p>

	<div class="sr-track" class:sr-track--wx={wx.length > 0} bind:clientWidth={trackW}>
		{#if wx.length > 0}
			<!-- Weather brackets over the stops an alert affects, on the SAME
			     axis as everything else (they used to be index-spaced). -->
			<div class="sr-lane sr-wx-lane">
				{#each wx as b (b.w.id)}
					<span
						class="sr-wx"
						style:left="{b.from}px"
						style:width="{Math.max(b.to - b.from, 8)}px"
						style:--wx="var(--color-wx-{b.w.tier})"
						title="{b.w.event}: stops {b.w.fromSeq}–{b.w.toSeq}"
					>
						<span class="sr-wx-label"><WxTierGlyph tier={b.w.tier as never} size={10} /> {b.w.shortCode}</span>
					</span>
				{/each}
			</div>
		{/if}
		<!-- LEAD lane -->
		<div class="sr-lane sr-lead-lane">
			{#each incidents as i (i.p.id)}
				<span
					class="sr-incident"
					style:left="{i.x}px"
					role="button"
					onclick={() => flyToChainage(i.p.chainageMeters)}
					onkeydown={enterActivates(() => flyToChainage(i.p.chainageMeters))}
					tabindex={tabKey === `i:${i.p.id}` ? 0 : -1}
					bind:this={refs[`i:${i.p.id}`]}
					onfocus={() => (focusKey = `i:${i.p.id}`)}
					title={i.p.label}
					aria-label="{i.p.tier.label}: {i.p.label}"
				>
					<RideTierGlyph tier={i.p.tier} size={10} />
				</span>
			{/each}
			{#if leadPx != null}
				<span
					class="sr-edge sr-edge--lead sr-anchor-{anchor(leadPx, (model.leadLabel + ' ' + leadAgeText).length, incidents.map((i) => i.x))}"
					class:sr-edge--stale={leadAge === 'stale'}
					style:left="{leadPx}px"
					role="button"
					onclick={() => flyToEdge(edges.lead)}
					onkeydown={enterActivates(() => flyToEdge(edges.lead))}
					tabindex={tabKey === 'lead' ? 0 : -1}
					bind:this={refs['lead']}
					onfocus={() => (focusKey = 'lead')}
					title={leadRead}
					aria-label="{model.leadLabel} at {edges.lead?.mile != null ? `mile ${edges.lead.mile.toFixed(1)}` : `stop ${edges.lead?.seq}`}, reported {leadAgeText} ago"
				>
					<span class="sr-chev sr-chev--down" aria-hidden="true"></span>
					<span class="sr-edge-text">{model.leadLabel}{leadAge === 'aging' || leadAge === 'stale' ? ` ${leadAgeText}` : ''}</span>
				</span>
			{/if}
		</div>

		<!-- COURSE lane -->
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div class="sr-lane sr-course" onclick={onCourseLaneClick} role="presentation">
			{#if band}
				<span class="sr-line sr-line--swept" style:left="0" style:width="{band.swept}px"></span>
				{#if band.fieldEnd != null}
					<span class="sr-line sr-line--field" style:left="{band.swept}px" style:width="{band.fieldEnd - band.swept}px"></span>
					<span class="sr-line sr-line--ahead" style:left="{band.fieldEnd}px" style:right="0"></span>
				{:else}
					<span class="sr-line sr-line--ahead" style:left="{band.swept}px" style:right="0"></span>
				{/if}
			{:else}
				<span class="sr-line" class:sr-line--ahead={phase === 'pre-start'} class:sr-line--neutral={phase !== 'pre-start'} style:left="0" style:right="0"></span>
			{/if}

			{#each gates as g (g.s.id)}
				<span
					class="sr-gate"
					class:sr-gate--fired={g.s.status === 'fired'}
					style:left="{g.x}px"
					role="button"
					onclick={() => flyToMile(g.s.mile)}
					onkeydown={enterActivates(() => flyToMile(g.s.mile))}
					tabindex={tabKey === `g:${g.s.id}` ? 0 : -1}
					bind:this={refs[`g:${g.s.id}`]}
					onfocus={() => (focusKey = `g:${g.s.id}`)}
					title="Shutoff {g.s.name} — {g.s.status === 'fired' ? 'FIRED' : 'armed'}"
					aria-label="Shutoff {g.s.name}, mile {g.s.mile?.toFixed(1)}, {g.s.status}">╫</span
				>
			{/each}

			{#each stops as s (s.id)}
				{#if s.dodged}
					<span class="sr-tick" style:left="{s.truePx}px" aria-hidden="true"></span>
				{/if}
				<span
					class="sr-stop"
					class:sr-stop--oo={s.outOfOrder}
					style:left="{s.drawPx}px"
					role="button"
					tabindex={tabKey === `s:${s.id}` ? 0 : -1}
					bind:this={refs[`s:${s.id}`]}
					onfocus={() => (focusKey = `s:${s.id}`)}
					onclick={() => onStopActivate?.(s.id)}
					onkeydown={(e) => {
						if (e.key === 'Enter' || e.key === ' ') {
							e.preventDefault();
							onStopActivate?.(s.id);
						}
					}}
					title="{s.label} — stop {s.seq}{s.mile != null ? `, mile ${s.mile.toFixed(1)}` : ''} — {s.state}{s.staffed ? ` — ${s.staffed} at stop` : ''}{s.outOfOrder ? ' — out of order' : ''}"
					aria-label="Stop {s.seq}, {s.label}{s.mile != null ? `, mile ${s.mile.toFixed(1)}` : ''}, {s.state}{s.staffed ? `, ${s.staffed} operator${s.staffed === 1 ? '' : 's'} at stop` : ''}{s.outOfOrder ? ', out of order' : ''}"
				>
					<span class="sr-stop-glyph" style:color={s.color} aria-hidden="true">{s.glyph}</span><span class="sr-stop-seq" aria-hidden="true">{s.seq}{#if s.staffed}<span class="sr-staff">·{s.staffed}</span>{/if}</span>
				</span>
			{/each}

			{#each clusters as c, ci (`${ci}:${c.members.join(',')}`)}
				{@const ids = membersOf(c)}
				{@const key = `c:${ci}:${c.members.join(',')}`}
				{#if ids.length === 1}
					{@const r = rosterById.get(ids[0])}
					{#if r}
						<button
							class="sr-pip {pipClass(r)}"
							class:sr-pip--sweepunit={r.isSweepUnit}
							style:left="{c.px}px"
							style:--pip={catColor(r.category)}
							tabindex={tabKey === key ? 0 : -1}
							bind:this={refs[key]}
							onfocus={() => (focusKey = key)}
							onclick={() => onRosterActivate?.(r.checkInId)}
							title={memberText(r)}
							aria-label="{r.tacticalCall || r.callsign}, {stationCategoryMeta[r.category as StationCategory]?.label ?? r.category}, {whereText(r)}{r.handPlaced ? ', position set by hand' : `, position ${ageText(r.positionAt, now)} old`}{r.age === 'stale' ? ', stale' : ''}{r.isSweepUnit ? ', sweep unit' : ''}"
						></button>
					{/if}
				{:else}
					<button
						class="sr-badge"
						style:left="{c.px}px"
						tabindex={tabKey === key ? 0 : -1}
						bind:this={refs[key]}
						onfocus={() => (focusKey = key)}
						onclick={(e) => activateCluster(c, e.currentTarget, key)}
						title="{ids.length} roster members here — click to list"
						aria-label="{ids.length} roster members near mile {(((c.fromPx + c.toPx) / 2 / Math.max(trackW, 1)) * (axis.totalMiles ?? 0)).toFixed(1)}"
					>{ids.length}</button>
				{/if}
			{/each}
		</div>

		<!-- SWEEP lane -->
		<div class="sr-lane sr-sweep-lane">
			{#if sweepPx != null}
				<span
					class="sr-edge sr-edge--sweep sr-anchor-{anchor(sweepPx, (model.sweepLabel + ' ' + sweepAgeText).length)}"
					class:sr-edge--stale={sweepAge === 'stale'}
					style:left="{sweepPx}px"
					role="button"
					onclick={() => flyToEdge(edges.sweep)}
					onkeydown={enterActivates(() => flyToEdge(edges.sweep))}
					tabindex={tabKey === 'sweep' ? 0 : -1}
					bind:this={refs['sweep']}
					onfocus={() => (focusKey = 'sweep')}
					title={sweepRead}
					aria-label="{model.sweepLabel} at {edges.sweep?.mile != null ? `mile ${edges.sweep.mile.toFixed(1)}` : `stop ${edges.sweep?.seq}`}, reported {sweepAgeText} ago"
				>
					<span class="sr-chev sr-chev--up" aria-hidden="true"></span>
					<span class="sr-edge-text">{model.sweepLabel} {sweepAgeText}</span>
				</span>
			{/if}
		</div>

		{#if panel}
			<!-- Stop names, where they fit. Where they don't, the name is in the
			     stop's title and accessible name — never overprinted. -->
			<div class="sr-lane sr-names-lane" aria-hidden="true">
				{#each stops as s (s.id)}
					{#if s.showName}
						<span class="sr-name sr-name--{s.nameAlign}" style:left="{s.drawPx}px" style:max-width="{s.nameMaxPx}px">{s.name}</span>
					{/if}
				{/each}
			</div>
		{/if}
	</div>

	<!-- READOUT lane: the sentence you read on the air -->
	<div class="sr-readout">
		<!-- The text gives way, never the controls: at 800px the readout ran
		     76px long and clipped "Sweep passed…", the one control here that
		     commits. Ordered by importance, so the scale drops off first. -->
		<div class="sr-reads">
		<span class="sr-read sr-read--lead" class:sr-read--aging={leadAge === 'aging'} class:sr-read--stale={leadAge === 'stale'} aria-hidden="true">▾ {leadRead}</span>
		<span class="sr-read sr-read--sweep" class:sr-read--aging={sweepAge === 'aging'} class:sr-read--stale={sweepAge === 'stale'} aria-hidden="true">▴ {sweepRead}</span>
		{#if spreadRead}
			<span
				class="sr-read sr-read--muted"
				class:sr-read--warn={edges.inverted}
				title={spreadTitle}
				aria-hidden="true">{spreadRead}</span
			>
		{/if}
		<span class="sr-read sr-read--muted sr-read--scale" aria-hidden="true">{scaleRead}</span>
		</div>
		{#if notNear > 0}
			<button class="sr-link" onclick={() => onRosterList?.()} title="On the roster but more than 400 m from the course line">
				{notNear} not near course
			</button>
		{/if}
		<!-- An SVG, not U+24D8: that codepoint is not in the Linux font stack and
		     rendered as a tofu box (the same reason STOP_GLYPHS avoids U+23F3). -->
		<button class="sr-legend-btn" bind:this={legendBtn} onclick={openLegend} aria-label="What the course rail shows" title="What the course rail shows">
			<svg width="13" height="13" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6.8" fill="none" stroke="currentColor" stroke-width="1.4" /><path d="M8 7.2v4.3M8 4.6v.1" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" /></svg>
		</button>
		{#if onSweepPassed}
			<!-- B7: a text button in the readout, not a lone ▲ glyph sitting on
			     top of the finish — it used to swallow clicks on the finish stop
			     and read as "sweep is at the finish". -->
			<button class="sr-sweep-btn" onclick={onSweepPassed}>{model.sweepLabel.charAt(0) + model.sweepLabel.slice(1).toLowerCase()} passed…</button>
		{/if}
	</div>
</div>

{#if popover?.kind === 'members'}
	<div class="sr-popover" use:topLayer style:left="{popover.x}px" style:bottom="{window.innerHeight - popover.y + 6}px" role="dialog" aria-label="Roster members here">
		<ul class="sr-pop-list">
			{#each popover.ids.slice(0, 8) as id (id)}
				{@const r = rosterById.get(id)}
				{#if r}
					<li>
						<button class="sr-pop-row" onclick={() => { closePopover(); onRosterActivate?.(id); }}>
							<span class="sr-pop-dot {pipClass(r)}" style:--pip={catColor(r.category)} aria-hidden="true"></span>
							<span class="sr-pop-text">{memberText(r)}</span>
						</button>
					</li>
				{/if}
			{/each}
		</ul>
		{#if popover.ids.length > 8}
			<button class="sr-pop-more" onclick={() => { closePopover(); onRosterList?.(); }}>+{popover.ids.length - 8} more — open roster</button>
		{/if}
	</div>
{:else if popover?.kind === 'legend'}
	<div class="sr-popover sr-legend" use:topLayer style:right="{window.innerWidth - popover.x}px" style:bottom="{window.innerHeight - popover.y + 6}px" role="dialog" aria-label="Course rail legend">
		<h3 class="sr-legend-h">Course rail</h3>
		<p class="sr-legend-p">Everything is placed along the course, in miles. {axis.mode === 'mile' ? `The rail is to scale: ${axis.totalMiles?.toFixed(1)} mi end to end.` : 'No course line is loaded, so stops are evenly spaced — not to scale.'}</p>
		<dl class="sr-legend-dl">
			<dt><span class="sr-chev sr-chev--down sr-lg-lead"></span></dt><dd>{model.leadLabel} — the front of the ride, from the last reported passage</dd>
			<dt><span class="sr-chev sr-chev--up sr-lg-sweep"></span></dt><dd>{model.sweepLabel} — the back of the ride, from the last report or passage. Never from GPS.</dd>
			<dt><span class="sr-lg-line sr-lg-line--field"></span></dt><dd>Between sweep and lead: where riders are</dd>
			<dt><span class="sr-lg-line sr-lg-line--swept"></span></dt><dd>Behind sweep: clear</dd>
			<dt><span class="sr-lg-line sr-lg-line--ahead"></span></dt><dd>Ahead of lead: not reached yet</dd>
			<dt><span class="sr-lg-glyph">{STOP_GLYPHS.open}2<span class="sr-staff">·3</span></span></dt><dd>Stop 2, open, 3 operators within 150 m of it</dd>
			<dt><span class="sr-lg-glyph">{STOP_GLYPHS.planned} {STOP_GLYPHS.awaitingSweep} {STOP_GLYPHS.readyToClose} {STOP_GLYPHS.atCapacity} {STOP_GLYPHS.closed}</span></dt><dd>Planned · awaiting sweep · sweep passed · at capacity · closed</dd>
			<dt><span class="sr-lg-glyph">╫</span></dt><dd>Shutoff gate (red once fired)</dd>
			<dt><span class="sr-pip sr-pip--fresh sr-lg-pip" style:--pip={stationCategoryMeta.sag.color}></span></dt><dd>Roster member near the course, in their category colour</dd>
			<dt><span class="sr-pip sr-pip--aging sr-lg-pip" style:--pip={stationCategoryMeta.sag.color}></span></dt><dd>Position 10–20 minutes old</dd>
			<dt><span class="sr-pip sr-pip--stale sr-lg-pip" style:--pip={stationCategoryMeta.sag.color}></span></dt><dd>Stale — where they were, over 20 minutes ago</dd>
			<dt><span class="sr-pip sr-pip--hand sr-lg-pip" style:--pip={stationCategoryMeta.sag.color}></span></dt><dd>Position set by hand</dd>
			<dt><span class="sr-pip sr-pip--ghost sr-lg-pip" style:--pip={stationCategoryMeta.sag.color}></span></dt><dd>On a road the course uses twice — shown at both miles</dd>
			<dt><span class="sr-badge sr-lg-badge">4</span></dt><dd>Several members close together — click to list them</dd>
		</dl>
		{#if notNear > 0}<p class="sr-legend-p">{notNear} roster member{notNear === 1 ? ' is' : 's are'} more than 400 m from the course and not shown.</p>{/if}
	</div>
{/if}

<style>
	.sr {
		container-type: inline-size;
		display: grid;
		grid-template-rows: auto var(--ride-rail-lane-readout);
		height: 100%;
		min-width: 0;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}

	/* ONE coordinate system: every layer is a child of this track and every
	   x is px from its left edge (B4). */
	.sr-track {
		position: relative;
		display: grid;
		grid-template-rows: var(--ride-rail-lane-edge) var(--ride-rail-lane-course) var(--ride-rail-lane-edge);
		min-width: 0;
	}

	.sr-lane {
		position: relative;
		min-width: 0;
	}

	/* --- panel density: the same lanes, taller, with names and weather --- */
	.sr--panel {
		--ride-rail-lane-edge: 16px;
		--ride-rail-lane-course: 28px;
		--ride-rail-lane-readout: 20px;
		--ride-rail-pip: 7px;
		min-width: 0;
		gap: var(--space-xs);
	}

	/* The panel has no strip padding around it: inset the track so an end
	   stop's number and name stay inside the rail. */
	.sr--panel {
		grid-template-rows: auto auto;
	}

	.sr--panel .sr-track {
		margin-inline: var(--space-md);
		grid-template-rows: var(--ride-rail-lane-edge) var(--ride-rail-lane-course) var(--ride-rail-lane-edge) 16px;
	}

	.sr--panel .sr-track.sr-track--wx {
		grid-template-rows: 18px var(--ride-rail-lane-edge) var(--ride-rail-lane-course) var(--ride-rail-lane-edge) 16px;
	}

	.sr--panel .sr-stop-glyph {
		font-size: 15px;
	}

	/* The panel's readout may take a second line; its row is sized to fit,
	   or the wrapped half (the roster count, the legend) was clipped away. */
	/* The panel readout WRAPS, text and all: at phone width the text group
	   refused to wrap and the sweep reading was clipped mid-number. */
	.sr--panel .sr-reads {
		flex-wrap: wrap;
		row-gap: 0;
		overflow: visible;
		white-space: normal;
	}

	.sr--panel .sr-read {
		white-space: nowrap;
	}

	.sr--panel .sr-readout {
		flex-wrap: wrap;
		row-gap: 0;
		white-space: normal;
		overflow: visible;
		padding-inline: var(--space-md);
	}

	.sr--presentation {
		--ride-t-body: 1rem;
		--ride-t-label: 0.8125rem;
		--ride-rail-lane-edge: 20px;
		--ride-rail-lane-course: 34px;
		--ride-rail-lane-readout: 24px;
		--ride-rail-pip: 9px;
	}

	.sr--presentation .sr-stop-glyph {
		font-size: 19px;
	}

	.sr-wx {
		position: absolute;
		bottom: 0;
		height: 7px;
		border: 2px solid var(--wx);
		border-bottom: none;
		border-radius: 3px 3px 0 0;
	}

	.sr-wx-label {
		position: absolute;
		bottom: 7px;
		left: 0;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2xs);
		font-size: var(--ride-t-label);
		font-weight: 700;
		line-height: 1;
		color: var(--wx);
		white-space: nowrap;
	}

	.sr-name {
		position: absolute;
		top: 1px;
		transform: translateX(-50%);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-align: center;
		font-size: var(--ride-t-label);
		line-height: 14px;
		color: var(--color-text-muted);
	}

	.sr-name--start {
		transform: translateX(-6px);
		text-align: left;
	}

	.sr-name--end {
		transform: translateX(calc(-100% + 6px));
		text-align: right;
	}

	/* --- line + field band --- */
	.sr-line {
		position: absolute;
		top: 50%;
		height: 2px;
		transform: translateY(-50%);
		background: var(--color-hairline);
	}

	.sr-line--neutral {
		background: var(--color-primary);
	}

	.sr-line--swept {
		background: var(--color-primary);
	}

	.sr-line--field {
		height: 4px;
		background: var(--color-text);
		opacity: 0.7;
		border-radius: 2px;
	}

	.sr-line--ahead {
		background: repeating-linear-gradient(
			to right,
			var(--color-text-muted) 0 4px,
			transparent 4px 8px
		);
		opacity: 0.6;
	}

	/* --- stops --- */
	.sr-stop {
		position: absolute;
		top: 50%;
		transform: translate(-50%, -50%);
		display: inline-flex;
		align-items: baseline;
		gap: 1px;
		padding: 0 var(--space-2xs);
		border-radius: var(--radius-sm);
		background: var(--color-surface);
		cursor: pointer;
		z-index: 2;
		line-height: 1;
	}

	.sr-stop:hover,
	.sr-stop:focus-visible {
		background: var(--color-primary);
	}

	.sr-stop--oo {
		outline: 1px dashed var(--color-warning);
	}

	.sr-stop-glyph {
		font-size: 12px;
	}

	/* Out of flow, so the GLYPH is what sits on the stop's mile — centring
	   glyph+number together put the glyph 3.6px left of the mile, and the
	   LEAD/SWEEP chevrons (which are exactly on it) visibly missed the stop. */
	.sr-stop-seq {
		position: absolute;
		left: 100%;
		top: 50%;
		transform: translateY(-50%);
		padding: 0 1px;
		border-radius: 2px;
		background: var(--color-surface);
		font-size: var(--ride-t-label);
		font-weight: 700;
		line-height: 1;
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sr-staff {
		font-weight: 400;
	}

	.sr-tick {
		position: absolute;
		top: 50%;
		width: 1px;
		height: 8px;
		transform: translate(-50%, -50%);
		background: var(--color-text-muted);
		opacity: 0.7;
	}

	/* --- gates --- */
	.sr-gate {
		position: absolute;
		top: 50%;
		transform: translate(-50%, -50%);
		font-size: 14px;
		font-weight: 700;
		line-height: 1;
		color: var(--color-text-muted);
		z-index: 1;
	}

	.sr-gate--fired {
		color: var(--color-error);
	}

	/* --- roster pips --- */
	.sr-pip {
		position: absolute;
		top: 50%;
		width: var(--ride-rail-pip);
		height: var(--ride-rail-pip);
		padding: 0;
		margin: 0;
		transform: translate(-50%, -50%);
		border: none;
		border-radius: 50%;
		background: var(--pip);
		cursor: pointer;
		z-index: 3;
	}

	/* The pip is 6px; the hit area is the lane's full height. */
	.sr-pip::before {
		content: '';
		position: absolute;
		inset: -7px -5px;
	}

	.sr-pip--aging {
		box-shadow: 0 0 0 1px var(--color-surface), 0 0 0 2px var(--color-warning);
	}

	.sr-pip--stale {
		background: transparent;
		border: 1px dashed var(--pip);
		opacity: 0.6;
	}

	.sr-pip--hand {
		border-radius: 1px;
	}

	.sr-pip--ghost {
		background: transparent;
		border: 1px dotted var(--pip);
	}

	.sr-pip:focus-visible,
	.sr-badge:focus-visible,
	.sr-edge:focus-visible,
	.sr-gate:focus-visible,
	.sr-incident:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}

	.sr-badge {
		position: absolute;
		top: 50%;
		transform: translate(-50%, -50%);
		min-width: 14px;
		height: 14px;
		padding: 0 var(--space-xs);
		border: 1px solid var(--color-text-muted);
		border-radius: var(--radius-full);
		/* Opaque: it sits ON the field band, which showed through the numeral. */
		background: var(--color-surface);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		font-weight: 700;
		line-height: 12px;
		font-variant-numeric: tabular-nums;
		cursor: pointer;
		z-index: 3;
	}

	/* --- lead / sweep --- */
	.sr-edge {
		position: absolute;
		top: 0;
		display: inline-flex;
		flex-direction: row;
		align-items: center;
		gap: var(--space-2xs);
		height: var(--ride-rail-lane-edge);
		/* Put the CHEVRON's centre (5px into the box) exactly on x. */
		transform: translateX(-5px);
		white-space: nowrap;
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		line-height: 1;
		cursor: pointer;
		z-index: 2;
	}

	.sr-anchor-before {
		flex-direction: row-reverse;
		transform: translateX(calc(-100% + 5px));
	}

	.sr-anchor-bare .sr-edge-text {
		display: none;
	}

	.sr-edge--lead {
		color: var(--color-ride-lead);
		/* ▾ sits on the lane's floor, pointing down at the line. */
		align-items: flex-end;
	}

	.sr-edge--sweep {
		color: var(--color-ride-sweep);
		/* ▴ sits on the lane's ceiling, pointing up at the line. */
		align-items: flex-start;
	}

	.sr-edge--stale {
		opacity: 0.6;
	}

	/* Chevrons POINT AT THE LINE: ▾ from above for lead, ▴ from below for
	   sweep — and the readout uses the same two symbols (B14). */
	.sr-chev {
		display: inline-block;
		width: 0;
		height: 0;
		border-left: 5px solid transparent;
		border-right: 5px solid transparent;
	}

	.sr-chev--down {
		border-top: 6px solid currentColor;
	}

	.sr-chev--up {
		border-bottom: 6px solid currentColor;
	}

	.sr-incident {
		position: absolute;
		top: 1px;
		transform: translateX(-50%);
		line-height: 0;
		z-index: 1;
	}

	/* --- readout --- */
	.sr-readout {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		font-size: var(--ride-t-body);
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		line-height: var(--ride-rail-lane-readout);
	}

	.sr-read--lead {
		color: var(--color-ride-lead);
	}

	.sr-read--sweep {
		color: var(--color-ride-sweep);
	}

	.sr-read--aging {
		text-decoration: underline dotted var(--color-warning);
		text-underline-offset: 3px;
	}

	.sr-read--stale {
		opacity: 0.7;
	}

	.sr-read--muted {
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.sr-read--warn {
		color: var(--color-warning);
		font-weight: 700;
	}

	.sr-reads {
		display: flex;
		align-items: center;
		gap: var(--space-md);
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Narrow rail: drop the scale outright rather than ellipsise it to "co…".
	   It is still in the legend and the spoken summary. */
	@container (max-width: 900px) {
		.sr-read--scale {
			display: none;
		}
	}

	.sr-reads > .sr-read:last-child {
		overflow: hidden;
		text-overflow: ellipsis;
		min-width: 0;
	}

	.sr-readout > :not(.sr-reads) {
		flex-shrink: 0;
	}

	.sr-link,
	.sr-legend-btn,
	.sr-sweep-btn {
		position: relative;
		background: none;
		border: none;
		padding: 0 var(--space-xs);
		color: var(--color-text-muted);
		font: inherit;
		font-size: var(--ride-t-label);
		font-weight: 700;
		line-height: var(--ride-rail-lane-readout);
		cursor: pointer;
	}

	/* 16px tall visually, 44px to a finger: the empty lane above is the pad. */
	.sr-link::before,
	.sr-legend-btn::before,
	.sr-sweep-btn::before {
		content: '';
		position: absolute;
		inset: -28px 0 0 0;
	}

	.sr-link {
		text-decoration: underline dotted;
		text-underline-offset: 3px;
	}

	.sr-legend-btn {
		display: inline-flex;
		align-items: center;
	}

	.sr-sweep-btn {
		color: var(--color-ride-sweep);
		border: 1px solid currentColor;
		border-radius: var(--radius-sm);
		line-height: 14px;
	}

	.sr-link:hover,
	.sr-legend-btn:hover {
		color: var(--color-text);
	}

	/* --- popovers (top layer) --- */
	.sr-popover {
		position: fixed;
		inset: auto;
		margin: 0;
		transform: translateX(-50%);
		max-width: min(420px, 92vw);
		padding: var(--space-xs);
		background: var(--color-surface);
		color: var(--color-text);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		font-size: var(--ride-t-body);
	}

	.sr-legend {
		transform: none;
		padding: var(--space-sm) var(--space-md);
	}

	.sr-pop-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.sr-pop-row,
	.sr-pop-more {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}

	.sr-pop-row:hover,
	.sr-pop-more:hover {
		background: var(--color-primary);
	}

	.sr-pop-dot {
		position: relative;
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--pip);
	}

	.sr-pop-text {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.sr-pop-more {
		color: var(--color-text-muted);
	}

	.sr-legend-h {
		margin: 0 0 var(--space-xs);
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
	}

	.sr-legend-p {
		margin: var(--space-xs) 0;
		color: var(--color-text-muted);
		white-space: normal;
	}

	.sr-legend-dl {
		display: grid;
		grid-template-columns: 56px 1fr;
		gap: var(--space-xs) var(--space-sm);
		align-items: center;
		margin: var(--space-sm) 0 0;
	}

	.sr-legend-dl dt {
		display: flex;
		justify-content: center;
		align-items: center;
		position: relative;
		height: 16px;
	}

	.sr-legend-dl dd {
		margin: 0;
		white-space: normal;
	}

	.sr-lg-lead {
		color: var(--color-ride-lead);
	}

	.sr-lg-sweep {
		color: var(--color-ride-sweep);
	}

	.sr-lg-line {
		display: block;
		width: 40px;
		height: 2px;
	}

	.sr-lg-line--field {
		height: 4px;
		background: var(--color-text);
		opacity: 0.7;
	}

	.sr-lg-line--swept {
		background: var(--color-primary);
	}

	.sr-lg-line--ahead {
		background: repeating-linear-gradient(to right, var(--color-text-muted) 0 4px, transparent 4px 8px);
	}

	.sr-lg-glyph {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.sr-lg-pip,
	.sr-lg-badge {
		position: static;
		transform: none;
	}
</style>
