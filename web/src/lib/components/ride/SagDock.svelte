<script lang="ts">
	// The SAG dock (docs/sag-map-spec.md §1, §2, §8, §10; docs/sag-dock-design.md)
	// — the persistent list beside the map on desktop/tablet, and the bottom
	// sheet's `sag` content on a phone.
	//
	// The dock's most important job is the group it renders FIRST: a request
	// whose pickup cannot be drawn is invisible on every other surface in the
	// app, so `⚑ NOT ON THE MAP` sits above everything, whatever the tier of
	// what is below it. A placeable EMERGENCY is already screaming on the map;
	// an unplaceable PRIORITY has nowhere else to be seen. [spec §2]
	//
	// Its second job is the radio call: "SAG 2, on scene". Every active leg
	// renders as a leg row with ONE primary button naming the vehicle and the
	// state it moves to; skips, rider exceptions and release sit behind `⋯`
	// and always confirm. Advances are irreversible on the server, so the
	// design leans on the label saying exactly what will be written, a
	// post-commit lock, and confirm buttons that never land under the finger
	// that opened them. (sag-dock-design.md, S7.)
	//
	// Keyboard model is the ride strip's verbatim: ONE tab stop, roving
	// tabindex, `role="toolbar"`. Thirty cards must not be thirty tab stops
	// between the map and the panel, so the per-card action buttons are
	// `tabindex="-1"` and are reached by named keys, with the keys named in
	// each item's screen-reader description.
	import { tick } from 'svelte';
	import { sagPlaced, sagUnplaced, sagVehicleGeo, sagCourseNotice, sagRoute, sagFocus, focusSagRequest, focusSagVehicle, startDispatchFocus, type PlacedRequest } from '$lib/stores/sagMap';
	import { sagBoard, rideLadder, navigateZone, sagEditorSeed, upsertSagRequest } from '$lib/stores/ride';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { sagFocusPresetOn } from '$lib/stores/mapSettings';
	import { secondClock } from '$lib/stores/clock';
	import { canOperate } from '$lib/stores/session';
	import { showToast } from '$lib/stores/toast';
	import { api, ApiError } from '$lib/api';
	import {
		tierById, tierStyle, ageText, ageState, enumLabel,
		SAG_STATUS_LABELS, SAG_LEG_STATUS_LABELS, SAG_DISPOSITION_LABELS, SAG_LOCATION_KIND_LABELS,
		SAG_BIKE_LABELS, SAG_BIKE_GLYPHS, SAG_BIKE_SHORT, BIKE_ORDER
	} from '$lib/rideMeta';
	import {
		nextLegAction, legSkips, legExceptions, activeLegs, legStateSince, vehicleJobs,
		unitLabel, sortVehicles, findUnit, jobExtent, allSagExtent, vehicleExtent,
		loadDraft, loadPlan, loadSummary, deliverDraft,
		type LatLon, type LegAction, type LoadLine, type VehicleJob
	} from '$lib/sagDockModel';
	import type { SagVehicleGeo } from '$lib/sagDispatch';
	import type { SAGLeg, SAGLocation, SAGRequest } from '$lib/types';
	import RideTierGlyph from '../RideTierGlyph.svelte';
	import ReasonDialog from '../ReasonDialog.svelte';

	let {
		onPlaceRequest,
		onPlaceVehicle,
		onSagFocusPreset,
		onFitPoints,
		onCollapse,
		collapsed = false,
		onExpand
	}: {
		/** Tablet (769-1199px): the dock collapses to a 44px rail of
		 *  tier-coloured stubs so the map keeps the width it needs, and expands
		 *  as an overlay on a tap (spec §11). */
		collapsed?: boolean;
		onExpand?: () => void;
		/** Enter the map's existing pick mode for this request's pickup/dropoff.
		 *  The dock never implements picking; it only asks for it. */
		onPlaceRequest?: (requestId: string, which: 'pickup' | 'dropoff') => void;
		/** Same, for a checked-in vehicle that has never reported a position. */
		onPlaceVehicle?: (checkInId: string) => void;
		/** The map's one-click `SAG focus` preset (spec §4). Rendered only when
		 *  the host wires it — a button that does nothing is worse than none. */
		onSagFocusPreset?: () => void;
		/** Fit the map to these points, clear of the dock / rail / sheet. The
		 *  host owns the map and the padding; the dock never touches Leaflet. */
		onFitPoints?: (pts: LatLon[]) => void;
		onCollapse?: () => void;
	} = $props();

	const METERS_PER_MILE = 1609.344;
	/** How long a committed leg's button shows its ✓ and ignores presses. The
	 *  label re-renders to the NEXT rung under the operator's finger; without
	 *  this a double-tap advances twice. */
	const COMMIT_LOCK_MS = 1500;
	/** A freshly opened confirm ignores presses this long: the double-tap that
	 *  opened it must not also commit it. */
	const ARM_MS = 400;
	const VIEW_KEY = 'nymeria.sagDock.view';

	let netId = $derived($activeNetId);
	let clock = $derived($secondClock);

	function rank(p: PlacedRequest): number {
		return tierById($rideLadder, p.request.priority)?.rank ?? 99;
	}

	/** "What is blocked on ME, worst first" — the same ordering SagBoard uses,
	 *  so the two surfaces can never disagree about which request is worst. */
	function byUrgency(a: PlacedRequest, b: PlacedRequest): number {
		const blocked = (p: PlacedRequest) => (p.waiting > 0 ? 0 : 1);
		return (
			blocked(a) - blocked(b) ||
			rank(a) - rank(b) ||
			Date.parse(a.request.createdAt) - Date.parse(b.request.createdAt)
		);
	}

	let unplaced = $derived([...$sagUnplaced].sort(byUrgency));
	let waiting = $derived($sagPlaced.filter((p) => p.pickup.placed && p.waiting > 0).sort(byUrgency));
	let inMotion = $derived($sagPlaced.filter((p) => p.pickup.placed && p.waiting === 0).sort(byUrgency));
	/** Checked-in SAG units that have never reported a position. They are absent
	 *  from the map entirely (spec §3.5), so this group is their only home. */
	let noPosition = $derived(
		$sagVehicleGeo.filter((g) => g.vehicle.checkInStatus !== 'released' && (g.lat == null || g.lon == null))
	);
	let vehicleCount = $derived(($sagBoard?.vehicles ?? []).filter((v) => v.checkInStatus !== 'released').length);
	let ridersWaiting = $derived($sagPlaced.reduce((n, p) => n + p.waiting, 0));

	/** Vehicles view (S6): every checked-in unit in stable name order, each with
	 *  its active jobs. */
	let units = $derived(sortVehicles($sagVehicleGeo.filter((g) => g.vehicle.checkInStatus !== 'released').map((g) => ({ ...g, checkInId: g.vehicle.checkInId, tacticalCall: g.vehicle.tacticalCall, callsign: g.vehicle.callsign }))));
	let jobsByUnit = $derived(vehicleJobs($sagBoard?.requests ?? []));
	let placedById = $derived(new Map($sagPlaced.map((p) => [p.request.id, p])));
	let geoById = $derived(new Map($sagVehicleGeo.map((g) => [g.vehicle.checkInId, g])));

	// In motion is where S1–S3 happen, so it opens by default. It still sits
	// below Waiting: what is blocked on the operator leads.
	let motionOpen = $state(true);

	function readView(): 'requests' | 'vehicles' {
		try {
			return localStorage.getItem(VIEW_KEY) === 'vehicles' ? 'vehicles' : 'requests';
		} catch {
			return 'requests';
		}
	}
	let view = $state<'requests' | 'vehicles'>(readView());
	function setView(v: 'requests' | 'vehicles'): void {
		if (v !== view && listEl) listEl.scrollTop = 0;
		view = v;
		try {
			localStorage.setItem(VIEW_KEY, v);
		} catch {
			/* per-viewer convenience only */
		}
	}

	/** The rail's stubs: one per tier that actually has somebody waiting, worst
	 *  first, plus the unplaceable group — which keeps its position above every
	 *  tier here for the same reason it does in the full dock. Tier names are
	 *  never hardcoded; the ladder is agency-configurable. */
	let railStubs = $derived.by(() => {
		const byTier = new Map<string, { code: string; colorVar: string; rank: number; n: number }>();
		for (const p of waiting) {
			const t = tierById($rideLadder, p.request.priority);
			if (!t) continue;
			const st = tierStyle(t);
			const cur = byTier.get(t.id);
			if (cur) cur.n += 1;
			else byTier.set(t.id, { code: st.code, colorVar: st.colorVar, rank: t.rank, n: 1 });
		}
		return [...byTier.values()].sort((a, b) => a.rank - b.rank);
	});

	type DockItem =
		| { kind: 'request'; id: string; group: 'unplaced' | 'waiting' | 'motion'; p: PlacedRequest }
		| { kind: 'leg'; id: string; r: SAGRequest; leg: SAGLeg }
		| { kind: 'motion-toggle'; id: string }
		| { kind: 'vehicle'; id: string; g: SagVehicleGeo };

	function legItems(r: SAGRequest): DockItem[] {
		return activeLegs(r).map((leg) => ({ kind: 'leg' as const, id: `leg:${leg.id}`, r, leg }));
	}

	function reqItems(list: PlacedRequest[], group: 'unplaced' | 'waiting' | 'motion'): DockItem[] {
		return list.flatMap((p) => [{ kind: 'request' as const, id: p.request.id, group, p }, ...legItems(p.request)]);
	}

	/** The roving order, flattened exactly as the groups render below — the
	 *  arrow keys walk across group boundaries, so index and DOM order must be
	 *  the same list. */
	let items = $derived<DockItem[]>(
		view === 'vehicles'
			? units.flatMap((g) => [
					{ kind: 'vehicle' as const, id: `veh:${g.vehicle.checkInId}`, g },
					...(jobsByUnit.get(g.vehicle.checkInId) ?? []).map((j) => ({ kind: 'leg' as const, id: `leg:${j.leg.id}`, r: j.request, leg: j.leg }))
				])
			: [
					...reqItems(unplaced, 'unplaced'),
					...reqItems(waiting, 'waiting'),
					...(inMotion.length > 0 ? [{ kind: 'motion-toggle' as const, id: '__motion' }] : []),
					...(motionOpen ? reqItems(inMotion, 'motion') : []),
					...noPosition.map((g) => ({ kind: 'vehicle' as const, id: `veh:${g.vehicle.checkInId}`, g }))
				]
	);
	let indexOf = $derived(new Map(items.map((it, i) => [it.id, i])));

	let focusIdx = $state(0);
	let listEl = $state<HTMLElement | undefined>();

	// A roving index that outruns its list leaves NO item at tabindex 0 and the
	// whole dock silently drops out of the tab order — the exact failure the
	// single-tab-stop pattern exists to prevent. (RideStrip.svelte, same bug.)
	$effect(() => {
		const n = items.length;
		if (n > 0 && focusIdx > n - 1) focusIdx = n - 1;
	});

	function tabFor(id: string): 0 | -1 {
		return indexOf.get(id) === focusIdx ? 0 : -1;
	}

	function hits(): HTMLElement[] {
		return Array.from(listEl?.querySelectorAll<HTMLElement>('.sd-item') ?? []);
	}

	function moveTo(i: number): void {
		focusIdx = i;
		hits()[i]?.focus();
	}

	async function focusItem(id: string): Promise<void> {
		await tick();
		const i = indexOf.get(id);
		if (i != null) moveTo(i);
	}

	function onFocusIn(e: FocusEvent): void {
		const all = hits();
		const idx = all.indexOf((e.target as HTMLElement)?.closest('.sd-item') as HTMLElement);
		if (idx >= 0) focusIdx = idx;
	}

	// ---- zoom (S5) ----

	function fit(pts: LatLon[], none: string): void {
		if (pts.length === 0) {
			announce(none);
			return;
		}
		onFitPoints?.(pts);
	}

	function fitJob(requestId: string): void {
		const p = placedById.get(requestId);
		if (!p) return;
		fit(jobExtent(p, $sagVehicleGeo), `SAG ${p.request.sequence} is not on the map; the map stays put.`);
	}

	function fitVehicle(g: SagVehicleGeo): void {
		fit(vehicleExtent(g), `${unitLabel(g.vehicle)} has no position to zoom to.`);
	}

	function fitAll(): void {
		fit(allSagExtent($sagPlaced, $sagVehicleGeo), 'No SAG pickups or vehicles on the map.');
	}

	function selectRequest(p: PlacedRequest): void {
		focusSagRequest(p.request.id, 'select');
		fitJob(p.request.id);
	}

	function selectVehicle(g: SagVehicleGeo): void {
		focusSagVehicle(g.vehicle.checkInId);
		fitVehicle(g);
	}

	function activate(item: DockItem): void {
		switch (item.kind) {
			case 'motion-toggle':
				motionOpen = !motionOpen;
				return;
			case 'request':
				selectRequest(item.p);
				return;
			case 'vehicle':
				selectVehicle(item.g);
				return;
			case 'leg':
				primary(item.r, item.leg);
				return;
		}
	}

	function zoomItem(item: DockItem): void {
		if (item.kind === 'request') fitJob(item.p.request.id);
		else if (item.kind === 'leg') fitJob(item.r.id);
		else if (item.kind === 'vehicle') fitVehicle(item.g);
	}

	// ---- find a unit (S8) ----

	let findOpen = $state(false);
	let findQuery = $state('');
	let findEl = $state<HTMLInputElement | undefined>();
	let findHit = $derived(findQuery.trim() ? findUnit(findQuery, units) : null);
	let findHitLabel = $derived(findHit ? unitLabel(units.find((u) => u.checkInId === findHit)!.vehicle) : '');

	async function openFind(): Promise<void> {
		findOpen = true;
		findQuery = '';
		await tick();
		findEl?.focus();
	}

	async function onFindKey(e: KeyboardEvent): Promise<void> {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			findOpen = false;
			await tick();
			hits()[focusIdx]?.focus();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			e.stopPropagation();
			const id = findHit;
			if (!id) {
				announce(`No SAG unit matches ${findQuery}.`);
				return;
			}
			findOpen = false;
			setView('vehicles');
			const g = geoById.get(id);
			if (g) selectVehicle(g);
			await focusItem(`veh:${id}`);
			announce(`${findHitLabel || 'Unit'} found.`);
		}
	}

	// ---- leg actions (S1–S4, S7) ----

	/** One in-flight write at a time, keyed so its button shows the pending
	 *  state — the board's `busy` convention (SagBoard.svelte). */
	let busy = $state<string | null>(null);
	/** legId -> the ✓ text its button shows while locked after a commit. */
	let flash = $state<Record<string, string>>({});
	/** legId -> the server's refusal, shown on the leg until the next action. */
	let legError = $state<Record<string, string>>({});

	type Panel =
		| { kind: 'load'; legId: string; lines: LoadLine[]; change: boolean; skip: boolean }
		| { kind: 'deliver'; legId: string; slotIds: string[]; change: boolean; elsewhere: boolean; dest: SAGLocation }
		| { kind: 'more'; legId: string }
		| { kind: 'confirm'; legId: string; text: string; confirmLabel: string; run: () => Promise<void> };
	let panel = $state<Panel | null>(null);
	let panelOpenedAt = 0;
	let releaseTarget = $state<{ r: SAGRequest; leg: SAGLeg } | null>(null);

	function openPanel(p: Panel): void {
		panel = p;
		panelOpenedAt = Date.now();
		if (legError[p.legId]) legError = { ...legError, [p.legId]: '' };
		// A panel opened on the last card in view is otherwise below the fold.
		// Focus goes to its commit (or first) button: a keyboard operator must
		// be able to reach it, and the panel's controls are in the tab order
		// only while it is open. ARM_MS keeps a held Enter from committing.
		void tick().then(() => {
			const el = listEl?.querySelector<HTMLElement>('.sd-panel');
			if (!el) return;
			el.scrollIntoView({ block: 'nearest' });
			el.querySelector<HTMLElement>('.sd-act--commit, button')?.focus({ preventScroll: true });
		});
	}

	async function closePanel(refocusLeg = true): Promise<void> {
		const id = panel?.legId;
		panel = null;
		if (refocusLeg && id) await focusItem(`leg:${id}`);
	}

	function armed(): boolean {
		return Date.now() - panelOpenedAt >= ARM_MS;
	}

	function locked(legId: string): boolean {
		return busy !== null || !!flash[legId];
	}

	function vname(leg: SAGLeg): string {
		return leg.vehicleLabel || 'Vehicle';
	}

	function showFlash(legId: string, text: string): void {
		flash = { ...flash, [legId]: text };
		setTimeout(() => {
			const { [legId]: _, ...rest } = flash;
			flash = rest;
		}, COMMIT_LOCK_MS);
	}

	function fail(legId: string, e: unknown, fallback: string): void {
		const msg = e instanceof ApiError ? e.message : fallback;
		legError = { ...legError, [legId]: `${fallback} — ${msg}` };
		void tick().then(() => listEl?.querySelector(`[data-legerr="${legId}"]`)?.scrollIntoView({ block: 'nearest' }));
		showToast(msg, 'error');
		announce(`${fallback}: ${msg}`);
	}

	async function run(key: string, fn: () => Promise<void>): Promise<void> {
		if (busy) return;
		busy = key;
		try {
			await fn();
		} finally {
			busy = null;
		}
	}

	/** The leg's one primary: advance, or open the load/deliver confirm. */
	function primary(r: SAGRequest, leg: SAGLeg): void {
		if (!$canOperate || locked(leg.id)) return;
		if (legError[leg.id]) legError = { ...legError, [leg.id]: '' };
		const a = nextLegAction(leg.status);
		if (!a) return;
		if (a.verb === 'advance') void advance(r, leg, a, false);
		else if (a.verb === 'load') openLoad(r, leg, false);
		// Idempotent: a second press on an already-open confirm leaves it open
		// rather than toggling it shut under a double-tap.
		else if (panel?.kind !== 'deliver' || panel.legId !== leg.id) openDeliver(r, leg);
	}

	async function advance(r: SAGRequest, leg: SAGLeg, a: LegAction, skipped: boolean): Promise<void> {
		const to = a.to as 'enroute' | 'onscene';
		await run(`adv-${leg.id}`, async () => {
			try {
				upsertSagRequest(await api.advanceSagLeg(netId, r.id, leg.id, to));
				if (panel?.legId === leg.id) panel = null;
				showFlash(leg.id, `✓ ${vname(leg)} ${a.label.toLowerCase()}`);
				announce(`${vname(leg)} ${a.label.toLowerCase()}${skipped ? ' (skipped ahead)' : ''}, SAG ${r.sequence}.`);
			} catch (e) {
				fail(leg.id, e, `Could not mark ${vname(leg)} ${a.label.toLowerCase()}`);
			}
		});
	}

	function openLoad(r: SAGRequest, leg: SAGLeg, skip: boolean): void {
		if (panel?.kind === 'load' && panel.legId === leg.id) return;
		openPanel({ kind: 'load', legId: leg.id, lines: loadDraft(r, leg), change: false, skip });
	}

	async function submitLoad(r: SAGRequest, leg: SAGLeg): Promise<void> {
		if (panel?.kind !== 'load' || !armed()) return;
		const plan = loadPlan(panel.lines);
		if (plan.slotIds.length === 0) return;
		await run(`load-${leg.id}`, async () => {
			try {
				let updated = await api.loadSagSlots(netId, r.id, leg.id, { slotIds: plan.slotIds, bike: plan.bike });
				upsertSagRequest(updated);
				// A rider who refused: the load detached them back to waiting,
				// which is what makes this resolve legal.
				for (const sid of plan.decline) {
					updated = await api.resolveSagSlot(netId, r.id, sid, { disposition: 'declined' });
					upsertSagRequest(updated);
				}
				panel = null;
				const n = plan.slotIds.length;
				showFlash(leg.id, `✓ ${vname(leg)} loaded`);
				announce(`${vname(leg)} loaded, ${n} rider${n === 1 ? '' : 's'}, SAG ${r.sequence}.`);
			} catch (e) {
				fail(leg.id, e, `Could not record the load on ${vname(leg)}`);
			}
		});
	}

	function openDeliver(r: SAGRequest, leg: SAGLeg): void {
		openPanel({ kind: 'deliver', legId: leg.id, slotIds: deliverDraft(r, leg), change: false, elsewhere: false, dest: { ...r.dropoff } });
	}

	function dropoffName(r: SAGRequest): string {
		const p = placedById.get(r.id);
		if (p?.dropoff.placed && p.dropoff.label) return p.dropoff.label;
		return r.dropoff.description?.trim() || enumLabel(r.dropoff.kind, SAG_LOCATION_KIND_LABELS) || 'the dropoff';
	}

	async function submitDeliver(r: SAGRequest, leg: SAGLeg): Promise<void> {
		if (panel?.kind !== 'deliver' || !armed() || panel.slotIds.length === 0) return;
		const { slotIds, elsewhere, dest } = panel;
		const where = elsewhere ? enumLabel(dest.kind, SAG_LOCATION_KIND_LABELS) : dropoffName(r);
		await run(`deliver-${leg.id}`, async () => {
			try {
				upsertSagRequest(await api.deliverSagSlots(netId, r.id, leg.id, { slotIds, destination: elsewhere ? dest : undefined }));
				panel = null;
				showFlash(leg.id, `✓ ${vname(leg)} delivered`);
				// The card usually leaves the dock with this write, so the toast
				// is the feedback that survives it.
				showToast(`${vname(leg)} delivered SAG ${r.sequence} at ${where}`, 'success');
				announce(`${vname(leg)} delivered SAG ${r.sequence} at ${where}.`);
			} catch (e) {
				fail(leg.id, e, `Could not record the delivery by ${vname(leg)}`);
			}
		});
	}

	function confirmSkip(r: SAGRequest, leg: SAGLeg, a: LegAction): void {
		if (a.verb === 'load') {
			openLoad(r, leg, true);
			return;
		}
		const cur = (SAG_LEG_STATUS_LABELS[leg.status] ?? leg.status).toLowerCase();
		openPanel({
			kind: 'confirm',
			legId: leg.id,
			text: `Mark ${vname(leg)} ${a.label.toLowerCase()}, skipping ahead from ${cur}? This cannot be undone.`,
			confirmLabel: `${vname(leg)} → ${a.label}`,
			run: () => advance(r, leg, a, true)
		});
	}

	function confirmResolve(r: SAGRequest, leg: SAGLeg, slotId: string, name: string, disposition: string): void {
		const others = r.slots.filter((s) => s.legId === leg.id && s.id !== slotId && (s.disposition === 'waiting' || s.disposition === 'loaded')).length;
		const consequence = others === 0 ? `This frees ${vname(leg)}.` : `${vname(leg)} keeps its other ${others === 1 ? 'rider' : `${others} riders`}.`;
		openPanel({
			kind: 'confirm',
			legId: leg.id,
			text: `${name}: ${SAG_DISPOSITION_LABELS[disposition] ?? disposition}? ${consequence}`,
			confirmLabel: `${name} ${EXC_SHORT[disposition] ?? disposition}`,
			run: async () => {
				await run(`resolve-${leg.id}`, async () => {
					try {
						upsertSagRequest(await api.resolveSagSlot(netId, r.id, slotId, { disposition }));
						panel = null;
						const msg = `${name} on SAG ${r.sequence}: ${(SAG_DISPOSITION_LABELS[disposition] ?? disposition).toLowerCase()}`;
						showToast(msg, 'success');
						announce(`${msg}.`);
					} catch (e) {
						fail(leg.id, e, `Could not resolve ${name}`);
					}
				});
			}
		});
	}

	async function submitConfirm(): Promise<void> {
		if (panel?.kind !== 'confirm' || !armed()) return;
		await panel.run();
	}

	async function submitRelease(reason: string): Promise<void> {
		const t = releaseTarget;
		if (!t) return;
		try {
			upsertSagRequest(await api.releaseSagLeg(netId, t.r.id, t.leg.id, reason));
			releaseTarget = null;
			showToast(`${vname(t.leg)} released`, 'success');
			announce(`${vname(t.leg)} released from SAG ${t.r.sequence}.`);
		} catch (e) {
			throw new Error(e instanceof ApiError ? e.message : 'Could not release vehicle');
		}
	}

	function openMore(leg: SAGLeg): void {
		if (!$canOperate) return;
		if (panel?.kind === 'more' && panel.legId === leg.id) {
			void closePanel();
			return;
		}
		openPanel({ kind: 'more', legId: leg.id });
	}

	// ---- keyboard ----

	async function onKey(e: KeyboardEvent): Promise<void> {
		const target = e.target as HTMLElement;
		// Inside an open confirm (its buttons, selects, checkboxes), the native
		// controls own the keys. Only Escape is ours: close it, back to the leg.
		if (target?.closest('.sd-panel') || ['INPUT', 'SELECT', 'TEXTAREA'].includes(target?.tagName)) {
			if (e.key === 'Escape' && panel) {
				e.preventDefault();
				e.stopPropagation();
				await closePanel();
			}
			return;
		}
		const n = items.length;
		const item = items[focusIdx];
		switch (e.key) {
			case 'ArrowDown':
				if (n) moveTo((focusIdx + 1) % n);
				break;
			case 'ArrowUp':
				if (n) moveTo((focusIdx - 1 + n) % n);
				break;
			case 'Home':
				if (n) moveTo(0);
				break;
			case 'End':
				if (n) moveTo(n - 1);
				break;
			case 'Enter':
			case ' ':
				if (item) activate(item);
				break;
			case 'z':
				if (item) zoomItem(item);
				break;
			case 'Z':
				fitAll();
				break;
			case 'o':
			case 'O':
				if (item?.kind === 'leg') openMore(item.leg);
				break;
			case 'v':
			case 'V': {
				setView(view === 'vehicles' ? 'requests' : 'vehicles');
				announce(view === 'vehicles' ? 'Vehicles view.' : 'Requests view.');
				await tick();
				moveTo(Math.min(focusIdx, Math.max(0, items.length - 1)));
				break;
			}
			case 'f':
			case 'F':
				void openFind();
				break;
			case 'd':
			case 'D':
				if (item?.kind === 'request' && item.p.waiting > 0) startDispatchFocus(item.p.request.id);
				break;
			case 'p':
			case 'P':
				if (item?.kind === 'request') onPlaceRequest?.(item.p.request.id, 'pickup');
				else if (item?.kind === 'vehicle') onPlaceVehicle?.(item.g.vehicle.checkInId);
				break;
			case 'm':
			case 'M':
				if (item?.kind === 'request') addMile(item.p);
				break;
			case 'i':
			case 'I':
				if (item?.kind === 'request') navigateZone('course-import');
				break;
			// Escape unwinds one layer at a time: an open confirm first, then
			// the tablet overlay. The collapse control is tabindex="-1" like
			// every other action here, so without the key a keyboard-only
			// operator could OPEN the dock and never shut it.
			case 'Escape':
				if (panel) {
					await closePanel();
					break;
				}
				if (!onCollapse) return;
				onCollapse();
				break;
			default:
				return;
		}
		e.preventDefault();
		// The strip's global single-letter keys (`a` acknowledges an emergency)
		// must never fire from inside the dock.
		e.stopPropagation();
	}

	/** `Add mile` — the faster repair when the caller is still on the air
	 *  saying a number. The mile-marker field lives in the request editor on
	 *  the SAG board, so this seeds that editor open on this request with the
	 *  caret already in the mile input, then navigates to the board. */
	function addMile(p: PlacedRequest): void {
		focusSagRequest(p.request.id, 'select');
		sagEditorSeed.set({ requestId: p.request.id, focusField: 'pickupMile' });
		navigateZone('sag');
	}

	function riderWord(n: number): string {
		return n === 1 ? '1 rider' : `${n} riders`;
	}

	function reqLabel(r: SAGRequest): string {
		return `SAG ${r.sequence}`;
	}

	function courseMiles(): string {
		const total = $sagRoute.index?.totalMeters;
		return total ? (total / METERS_PER_MILE).toFixed(0) : '';
	}

	/** §2's per-reason copy. The card states the reason IN WORDS — "not drawn"
	 *  is never left to be inferred from an absence. */
	function reasonText(p: PlacedRequest): string {
		if (p.pickup.placed) return '';
		const label = p.pickup.label;
		switch (p.pickup.reason) {
			case 'no-location':
				return 'Not on the map — no mile marker and no coordinate';
			case 'no-course':
				return `No course loaded — ${label || 'this mile marker'} can't be drawn`;
			case 'route-unknown':
				return p.request.pickup.route
					? `Route "${p.request.pickup.route}" isn't loaded`
					: `Several courses are loaded and nothing says which one ${label || 'this'} is on`;
			case 'off-route': {
				const len = courseMiles();
				return len
					? `${label} is past the end of the ${len} mi course`
					: `${label} is past the end of the course`;
			}
			default:
				return 'Not on the map';
		}
	}

	/** Which of §2's actions this reason offers, in the spec's order. */
	function actionsFor(p: PlacedRequest): ('place' | 'mile' | 'import')[] {
		if (p.pickup.placed) return [];
		switch (p.pickup.reason) {
			case 'no-course':
			case 'route-unknown':
				return ['import', 'place'];
			case 'off-route':
				return ['mile', 'place'];
			default:
				return ['place', 'mile'];
		}
	}

	function legPhrase(leg: SAGLeg): string {
		return `${vname(leg)} ${(SAG_LEG_STATUS_LABELS[leg.status] ?? leg.status).toLowerCase()}`;
	}

	function requestAria(p: PlacedRequest, group: string): string {
		const tier = tierById($rideLadder, p.request.priority);
		const legs = activeLegs(p.request);
		const parts = [
			tier ? tierStyle(tier).ariaWord : '',
			reqLabel(p.request),
			p.pickup.placed ? p.pickup.label : 'not on the map',
			p.waiting > 0 ? `${riderWord(p.waiting)} needing a vehicle` : enumLabel(p.request.status, SAG_STATUS_LABELS),
			legs.length ? legs.map(legPhrase).join(', ') : '',
			`reported ${ageText(p.request.createdAt, clock)} ago`
		];
		if (!p.pickup.placed) parts.push(reasonText(p));
		if (group === 'unplaced') parts.push('Press p to place it on the map, m to add a mile marker');
		else {
			parts.push('Enter or z to zoom the map to this job');
			if (p.waiting > 0) parts.push('d to find a driver');
		}
		return parts.filter(Boolean).join(', ');
	}

	function legAria(r: SAGRequest, leg: SAGLeg, inVehicleView: boolean): string {
		const a = nextLegAction(leg.status);
		const p = placedById.get(r.id);
		const where = p?.pickup.label || '';
		const head = inVehicleView
			? `${reqLabel(r)}${where ? `, ${where}` : ''}, ${legPhrase(leg)} ${ageText(legStateSince(leg), clock)}`
			: `${legPhrase(leg)} ${ageText(legStateSince(leg), clock)}, on ${reqLabel(r)}`;
		const act = !$canOperate || !a
			? ''
			: a.needsConfirm
				? `Enter to record ${vname(leg)} ${a.label.toLowerCase()}, with a confirm`
				: `Enter to mark ${vname(leg)} ${a.label.toLowerCase()}`;
		return [head, act, $canOperate ? 'o for more actions' : '', 'z to zoom to the job'].filter(Boolean).join('. ');
	}

	function posText(g: SagVehicleGeo): { text: string; state: string } {
		if (g.lat == null || g.lon == null) return { text: 'no fix', state: 'never' };
		const st = ageState(g.lastHeard, clock);
		const age = ageText(g.lastHeard, clock);
		if (st === 'never') return { text: 'fix, age unknown', state: 'stale' };
		return { text: st === 'stale' ? `stale ${age}` : age, state: st };
	}

	function vehicleAria(g: SagVehicleGeo): string {
		const v = g.vehicle;
		const jobs = jobsByUnit.get(v.checkInId) ?? [];
		const pos = posText(g);
		const posWords =
			pos.state === 'never' ? 'position never set' : pos.state === 'stale' ? `position stale, last fix ${ageText(g.lastHeard, clock)} ago` : `position ${ageText(g.lastHeard, clock)} old`;
		return [
			`SAG unit ${unitLabel(v)}`,
			jobs.length === 0 ? 'free' : `${jobs.length} job${jobs.length === 1 ? '' : 's'}`,
			`${v.availableSeats} of ${v.seats} seats free, ${v.availableRacks} of ${v.rackSlots} racks free`,
			posWords,
			pos.state === 'never' ? 'Press p to place it on the map' : 'Enter or z to zoom the map to it'
		].join(', ');
	}

	// --- one polite live region (spec §8) ---
	// Announced: a NEW request that needs a vehicle (batched to one a minute),
	// and the result of every action taken from the dock. Never `assertive` —
	// assertive belongs solely to the interrupt banner.
	let announcement = $state('');
	let lastAnnouncedAt = 0;
	let seen = new Set<string>();
	let primed = false;

	function announce(msg: string): void {
		// Re-announcing identical text is a no-op for most readers; clear first.
		announcement = '';
		void tick().then(() => (announcement = msg));
	}

	$effect(() => {
		const needy = $sagPlaced.filter((p) => p.waiting > 0).map((p) => p.request.id);
		if (!primed) {
			// The first pass is the existing board, not news.
			seen = new Set(needy);
			primed = true;
			return;
		}
		const fresh = needy.filter((id) => !seen.has(id));
		seen = new Set(needy);
		if (fresh.length === 0) return;
		const now = Date.now();
		if (now - lastAnnouncedAt < 60_000) return;
		lastAnnouncedAt = now;
		announcement =
			fresh.length === 1
				? `New SAG request needing a vehicle: ${fresh.length} waiting.`
				: `${fresh.length} new SAG requests needing a vehicle.`;
	});

	function tierVars(r: SAGRequest): string {
		const tier = tierById($rideLadder, r.priority);
		return tier ? `--sd-tier: var(${tierStyle(tier).colorVar}); --sd-tier-text: var(${tierStyle(tier).textVar});` : '';
	}

	/** Button-sized outcome words; the confirm then spells the outcome out. */
	const EXC_SHORT: Record<string, string> = {
		self_resolved: 'Rode on',
		declined: 'Declined',
		not_found: 'Not found',
		handed_off: 'To medical'
	};

	const EXC_VERB: Record<string, string> = {
		self_resolved: 'rode on (fixed it)',
		declined: 'declined SAG',
		not_found: 'not found on scene',
		handed_off: 'handed off to medical'
	};
</script>

<!-- One leg: the status line, the primary that names vehicle + target, the
     `⋯` More button at the far end, and whichever inline panel is open. The
     panel opens BELOW the action row, so the button that opened it stays put
     and a second tap lands on the same (idempotent) button, never on Confirm. -->
{#snippet legRow(r: SAGRequest, leg: SAGLeg, inVehicleView: boolean)}
	{@const a = nextLegAction(leg.status)}
	{@const lid = `leg:${leg.id}`}
	{@const open = panel?.legId === leg.id ? panel : null}
	{@const fl = flash[leg.id]}
	{@const p = placedById.get(r.id)}
	<div
		class="sd-item sd-leg"
		role="button"
		tabindex={tabFor(lid)}
		aria-label={legAria(r, leg, inVehicleView)}
		onclick={() => (p ? selectRequest(p) : focusSagRequest(r.id, 'select'))}
		onkeydown={() => {}}
		style={inVehicleView ? tierVars(r) : ''}
	>
		<div class="sd-leg-line">
			{#if inVehicleView}
				{@const tier = tierById($rideLadder, r.priority)}
				{#if tier}<span class="sd-code">{tierStyle(tier).code}</span>{/if}
				<span class="sd-name">{reqLabel(r)}</span>
				{#if p?.pickup.label}<span class="sd-sep">·</span><span class="sd-where">{p.pickup.label}</span>{/if}
			{:else}
				<span class="sd-unit" title="SAG unit">{vname(leg)}</span>
			{/if}
			<span class="sd-legstate" data-legstatus={leg.status}>{SAG_LEG_STATUS_LABELS[leg.status] ?? leg.status}</span>
			<span class="sd-legage" title="Time in this state">{ageText(legStateSince(leg), clock)}</span>
		</div>
		{#if $canOperate && a}
			<div class="sd-leg-actions">
				<button
					class="sd-act sd-act--primary sd-go"
					class:sd-go--done={!!fl}
					tabindex="-1"
					disabled={busy !== null && !fl}
					aria-disabled={!!fl}
					aria-busy={busy?.endsWith(leg.id) ?? false}
					aria-expanded={a.needsConfirm ? open?.kind === (a.verb === 'load' ? 'load' : 'deliver') : undefined}
					onclick={(e) => { e.stopPropagation(); primary(r, leg); }}
				>
					{#if fl}
						<span class="sd-go-text">{fl}</span>
					{:else if busy === `adv-${leg.id}`}
						<span class="sd-go-text">Saving…</span>
					{:else}
						<span class="sd-go-veh">{vname(leg)}</span>
						<span class="sd-go-to">→ {a.label}{a.needsConfirm ? '…' : ''}</span>
					{/if}
				</button>
				<button
					class="sd-act sd-more"
					tabindex="-1"
					aria-label={`More actions for ${vname(leg)} on ${reqLabel(r)} (o)`}
					title="More actions (o)"
					aria-expanded={open?.kind === 'more' || open?.kind === 'confirm'}
					disabled={busy !== null}
					onclick={(e) => { e.stopPropagation(); openMore(leg); }}
				>⋯</button>
			</div>
		{/if}
		{#if legError[leg.id]}
			<p class="sd-legerr" role="alert" data-legerr={leg.id}>⊘ {legError[leg.id]}</p>
		{/if}
	</div>

	{#if open}
		<div class="sd-panel" role="group" aria-label={`${vname(leg)} on ${reqLabel(r)}`}>
			{#if open.kind === 'load'}
				<p class="sd-panel-head">
					{open.skip ? `Skip ahead: load ${vname(leg)}` : `Load ${vname(leg)}`}
				</p>
				{#if open.lines.length === 0}
					<p class="sd-panel-note">Nobody on this leg is waiting to be loaded.</p>
				{:else}
					<p class="sd-panel-sum">{loadSummary(open.lines)}</p>
					{#if !open.change}
						<p class="sd-panel-note">{open.lines.map((l) => `${l.name} (${SAG_BIKE_SHORT[l.bike] ?? l.bike})`).join(' · ')}</p>
					{/if}
					<button class="sd-link" aria-expanded={open.change} onclick={() => { if (open.kind === 'load') open.change = !open.change; }}>
						{open.change ? 'Done changing' : 'Change who / which bike…'}
					</button>
					{#if open.change}
						{#each open.lines as line (line.slotId)}
							<div class="sd-loadline">
								<span class="sd-loadname">{line.name}</span>
								<select class="sd-select" bind:value={line.state} aria-label={`${line.name}: aboard?`}>
									<option value="aboard">Aboard</option>
									<option value="stays">Not aboard — stays waiting</option>
									<option value="declined">Declined SAG</option>
								</select>
								<select class="sd-select" bind:value={line.bike} disabled={line.state !== 'aboard'} aria-label={`Bike for ${line.name}`}>
									{#each BIKE_ORDER as b (b)}
										<option value={b}>{SAG_BIKE_GLYPHS[b]} {SAG_BIKE_LABELS[b]}</option>
									{/each}
								</select>
							</div>
						{/each}
					{/if}
				{/if}
				<div class="sd-panel-actions">
					<button
						class="sd-act sd-act--commit"
						tabindex="-1"
						disabled={busy !== null || loadPlan(open.lines).slotIds.length === 0}
						aria-busy={busy === `load-${leg.id}`}
						onclick={() => submitLoad(r, leg)}
					>{busy === `load-${leg.id}` ? 'Recording…' : 'Confirm load'}</button>
					<button class="sd-act" disabled={busy !== null} onclick={() => closePanel()}>Cancel</button>
				</div>
			{:else if open.kind === 'deliver'}
				{@const loaded = deliverDraft(r, leg)}
				<p class="sd-panel-head">Deliver {riderWord(open.slotIds.length)} at {open.elsewhere ? enumLabel(open.dest.kind, SAG_LOCATION_KIND_LABELS) : dropoffName(r)}</p>
				<button class="sd-link" aria-expanded={open.change} onclick={() => { if (open.kind === 'deliver') open.change = !open.change; }}>
					{open.change ? 'Done changing' : 'Change who / where…'}
				</button>
				{#if open.change}
					{#each loaded as sid (sid)}
						{@const s = r.slots.find((x) => x.id === sid)}
						<label class="sd-check">
							<input
								type="checkbox"
								tabindex="-1"
								checked={open.slotIds.includes(sid)}
								onchange={(e) => {
									if (open.kind !== 'deliver') return;
									const on = (e.target as HTMLInputElement).checked;
									open.slotIds = on ? [...open.slotIds, sid] : open.slotIds.filter((x) => x !== sid);
								}}
							/>
							{s?.bib ? `Bib ${s.bib}` : s?.riderName || 'Rider'}
						</label>
					{/each}
					<label class="sd-check"><input type="checkbox" bind:checked={open.elsewhere} /> Somewhere other than {dropoffName(r)}</label>
					{#if open.elsewhere}
						<select class="sd-select" bind:value={open.dest.kind} aria-label="Where they were dropped">
							<option value="next_reststop">Next rest stop</option>
							<option value="reststop">Rest stop</option>
							<option value="finish">Finish</option>
							<option value="start">Start</option>
							<option value="hospital">Hospital</option>
							<option value="other">Other</option>
						</select>
					{/if}
				{/if}
				<div class="sd-panel-actions">
					<button
						class="sd-act sd-act--commit"
						tabindex="-1"
						disabled={busy !== null || open.slotIds.length === 0}
						aria-busy={busy === `deliver-${leg.id}`}
						onclick={() => submitDeliver(r, leg)}
					>{busy === `deliver-${leg.id}` ? 'Recording…' : 'Confirm delivery'}</button>
					<button class="sd-act" disabled={busy !== null} onclick={() => closePanel()}>Cancel</button>
				</div>
			{:else if open.kind === 'more'}
				{@const skips = legSkips(leg.status)}
				{@const exc = legExceptions(r, leg)}
				{#if skips.length}
					<h4 class="sd-panel-h">Skip ahead</h4>
					{#each skips as k (k.to)}
						<button class="sd-act sd-menu" onclick={() => confirmSkip(r, leg, k)}>{vname(leg)} → {k.label}{k.needsConfirm ? '…' : ''}</button>
					{/each}
				{/if}
				{#if exc.length}
					<h4 class="sd-panel-h">Rider outcome</h4>
					{#each exc as x (x.slotId)}
						<div class="sd-exc" role="group" aria-label={x.name}>
							<span class="sd-loadname">{x.name}</span>
							{#each x.options as d (d)}
								<button class="sd-act sd-exc-btn" aria-label={`${x.name} ${EXC_VERB[d] ?? d}…`} onclick={() => confirmResolve(r, leg, x.slotId, x.name, d)}>{EXC_SHORT[d] ?? d}</button>
							{/each}
						</div>
					{/each}
				{/if}
				{#if leg.status !== 'loaded'}
					<button class="sd-act sd-menu sd-menu--danger" onclick={() => { panel = null; releaseTarget = { r, leg }; }}>Release {vname(leg)}…</button>
				{/if}
				<button class="sd-act sd-menu sd-menu--quiet" onclick={() => { focusSagRequest(r.id, 'select'); navigateZone('sag'); }}>Open on the SAG board</button>
			{:else if open.kind === 'confirm'}
				<p class="sd-panel-head">{open.text}</p>
				<div class="sd-panel-actions">
					<button class="sd-act sd-act--commit" disabled={busy !== null} aria-busy={busy !== null} onclick={submitConfirm}>{busy ? 'Saving…' : open.confirmLabel}</button>
					<button class="sd-act" disabled={busy !== null} onclick={() => openPanel({ kind: 'more', legId: leg.id })}>Back</button>
				</div>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet requestCard(p: PlacedRequest, group: 'unplaced' | 'waiting' | 'motion')}
	{@const tier = tierById($rideLadder, p.request.priority)}
	{@const legs = activeLegs(p.request)}
	<div
		class="sd-card"
		class:sd-card--unplaced={group === 'unplaced'}
		class:sd-card--motion={group === 'motion'}
		class:sd-card--focused={$sagFocus?.requestId === p.request.id}
		style={tierVars(p.request)}
	>
		<div
			class="sd-item sd-card-hit"
			role="button"
			tabindex={tabFor(p.request.id)}
			aria-label={requestAria(p, group)}
			onclick={() => selectRequest(p)}
			onkeydown={() => {}}
		>
			<div class="sd-row">
				{#if tier}
					{#if group !== 'motion'}<RideTierGlyph {tier} size={13} />{/if}
					<span class="sd-code">{tierStyle(tier).code}</span>
				{/if}
				<span class="sd-name">{reqLabel(p.request)}</span>
				<span class="sd-sep">·</span>
				{#if group === 'unplaced'}
					<span class="sd-riders">{riderWord(p.waiting)}</span>
				{:else}
					<span class="sd-where">{p.pickup.label}</span>
				{/if}
				<span class="sd-age">{ageText(p.request.createdAt, clock)}</span>
			</div>
			{#if group === 'unplaced'}
				{#if p.request.pickup.description?.trim()}
					<!-- The caller's own words, verbatim and quoted. This is the
					     only description of where they are that exists. -->
					<p class="sd-quote">“{p.request.pickup.description.trim()}”</p>
				{/if}
				<p class="sd-reason">{reasonText(p)}</p>
				<div class="sd-actions">
					{#each actionsFor(p) as act (act)}
						{#if act === 'place'}
							<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate || !onPlaceRequest} onclick={(e) => { e.stopPropagation(); onPlaceRequest?.(p.request.id, 'pickup'); }}>
								Place on map <kbd>p</kbd>
							</button>
						{:else if act === 'mile'}
							<button class="sd-act" tabindex="-1" onclick={(e) => { e.stopPropagation(); addMile(p); }}>
								Add mile <kbd>m</kbd>
							</button>
						{:else}
							<button class="sd-act" tabindex="-1" onclick={(e) => { e.stopPropagation(); navigateZone('course-import'); }}>
								Import course <kbd>i</kbd>
							</button>
						{/if}
					{/each}
				</div>
			{:else if group === 'waiting'}
				<div class="sd-row sd-row--sub">
					<span class="sd-riders">{riderWord(p.waiting)}</span>
					{#if p.request.needsVehicle}<span class="sd-needs">needs a vehicle</span>{/if}
				</div>
				<div class="sd-actions">
					<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate} onclick={(e) => { e.stopPropagation(); startDispatchFocus(p.request.id); }}>
						Find a driver <kbd>d</kbd>
					</button>
				</div>
			{/if}
		</div>
		{#each legs as leg (leg.id)}
			{@render legRow(p.request, leg, false)}
		{/each}
	</div>
{/snippet}

{#snippet unitCard(g: SagVehicleGeo)}
	{@const v = g.vehicle}
	{@const jobs = jobsByUnit.get(v.checkInId) ?? []}
	{@const pos = posText(g)}
	<div class="sd-card sd-card--unit" class:sd-card--focused={$sagFocus?.vehicleCheckInId === v.checkInId}>
		<div
			class="sd-item sd-card-hit"
			role="button"
			tabindex={tabFor(`veh:${v.checkInId}`)}
			aria-label={vehicleAria(g)}
			onclick={() => (pos.state === 'never' ? onPlaceVehicle?.(v.checkInId) : selectVehicle(g))}
			onkeydown={() => {}}
		>
			<div class="sd-row">
				<span class="sd-unit sd-unit--big">{unitLabel(v)}</span>
				{#if v.tacticalCall}<span class="sd-call">{v.callsign}</span>{/if}
				<span class="sd-age sd-pos" data-pos={pos.state}>{pos.text}</span>
			</div>
			<div class="sd-row sd-row--sub">
				{#if jobs.length === 0}<span class="sd-pill sd-pill--free">Free</span>{/if}
				<span class:sd-cap-warn={v.availableSeats <= 0}>{v.availableSeats}/{v.seats} seats</span>
				<span class="sd-sep">·</span>
				<span class:sd-cap-warn={v.availableRacks <= 0}>{v.availableRacks}/{v.rackSlots} racks free</span>
				{#if v.availableSeats <= 0}<span class="sd-pill sd-pill--full">{v.availableSeats < 0 ? 'Over' : 'Full'}</span>{/if}
			</div>
			{#if pos.state === 'never'}
				<p class="sd-reason">Voice check-in, position never set</p>
				{#if onPlaceVehicle}
					<div class="sd-actions">
						<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate} onclick={(e) => { e.stopPropagation(); onPlaceVehicle?.(v.checkInId); }}>
							Place on map <kbd>p</kbd>
						</button>
					</div>
				{/if}
			{/if}
		</div>
		{#each jobs as j (j.leg.id)}
			{@render legRow(j.request, j.leg, true)}
		{/each}
	</div>
{/snippet}

{#if collapsed}
	<!-- The tablet rail (spec §11). It carries COUNTS, not content: enough to
	     know whether opening it is worth the width, never enough to make a
	     dispatch decision from — which is why the whole stub is one button that
	     opens the real dock. -->
	<button
		class="sd-rail"
		style="--sag-unit: var(--color-sag-unit, #f97316);"
		onclick={onExpand}
		aria-label={`Open the SAG dock. ${ridersWaiting === 0 ? 'Nobody waiting' : `${ridersWaiting} waiting`}${unplaced.length > 0 ? `, ${unplaced.length} not on the map` : ''}.`}
	>
		<span class="sd-rail-title" aria-hidden="true">SAG</span>
		{#if unplaced.length > 0}
			<span class="sd-rail-stub sd-rail-stub--flag" aria-hidden="true">
				<span class="sd-rail-code">⚑</span><span class="sd-rail-n">{unplaced.length}</span>
			</span>
		{/if}
		{#each railStubs as st (st.code)}
			<span class="sd-rail-stub" style={`--sd-tier: var(${st.colorVar});`} aria-hidden="true">
				<span class="sd-rail-code">{st.code}</span><span class="sd-rail-n">{st.n}</span>
			</span>
		{/each}
		{#if railStubs.length === 0 && unplaced.length === 0}
			<span class="sd-rail-quiet" aria-hidden="true">—</span>
		{/if}
		<span class="sd-rail-grip" aria-hidden="true">›</span>
	</button>
{:else}
<section class="sd" aria-label="SAG dock" style="--sag-unit: var(--color-sag-unit, #f97316); --sag-unit-soft: var(--color-sag-unit-soft, rgba(249, 115, 22, 0.14));">
	<header class="sd-head">
		{#if findOpen}
			<label class="sd-find">
				<span class="sr-only">Find a SAG unit by name</span>
				<input
					bind:this={findEl}
					bind:value={findQuery}
					onkeydown={onFindKey}
					onblur={() => (findOpen = false)}
					placeholder="Find unit…"
					autocomplete="off"
					spellcheck="false"
				/>
			</label>
			<span class="sd-find-hit" aria-live="polite">{findQuery.trim() ? (findHitLabel ? `→ ${findHitLabel}` : 'no match') : ''}</span>
		{:else}
			<span class="sd-title">SAG</span>
			{#if $sagBoard}
				<span class="sd-count">{ridersWaiting > 0 ? `${ridersWaiting} waiting` : 'nobody waiting'}</span>
			{/if}
			{#if onFitPoints && $sagBoard}
				<button
					class="sd-head-btn sd-fitall"
					tabindex="-1"
					onclick={fitAll}
					aria-label="Zoom the map to all SAG pickups and vehicles (Shift+Z)"
					title="Zoom to all SAG (Shift+Z)"
				><span aria-hidden="true">⤢</span> All SAG</button>
			{/if}
			{#if onCollapse}
				<button
					class="sd-head-btn sd-collapse"
					class:sd-collapse--solo={!onFitPoints}
					tabindex="-1"
					onclick={onCollapse}
					aria-label="Collapse the SAG dock (Escape)"
					title="Collapse (Esc)"
				>⌄</button>
			{/if}
		{/if}
	</header>

	{#if $sagBoard}
		<!-- Requests vs vehicles (S6). Radio calls are keyed by the DRIVER, so
		     the vehicle list is a first-class view, not a filter. -->
		<div class="sd-tabs" role="group" aria-label="Show SAG by">
			<button class="sd-tab" tabindex="-1" aria-pressed={view === 'requests'} title="Requests (v)" onclick={() => setView('requests')}>
				Requests <span class="sd-chip">{$sagPlaced.length}</span>
			</button>
			<button class="sd-tab" tabindex="-1" aria-pressed={view === 'vehicles'} title="Vehicles (v)" onclick={() => setView('vehicles')}>
				Vehicles <span class="sd-chip">{vehicleCount}</span>
			</button>
		</div>
	{/if}

	{#if $sagCourseNotice}
		<p class="sd-notice">{$sagCourseNotice}</p>
	{/if}

	{#if !$sagBoard}
		<p class="sd-muted">Loading SAG…</p>
	{:else}
		{#if vehicleCount === 0}
			<!-- An absence that matters: no units means every request below is
			     unanswerable, and that is a fact, not a quiet empty list. -->
			<p class="sd-warn">
				<span class="sd-warn-head">⚠ No SAG units checked in</span>
				Check in a unit with category “sag”.
			</p>
		{/if}

		{#if items.length === 0 && view === 'requests'}
			<!-- Muted, never green: absence is not verified good. -->
			<p class="sd-muted">Nobody is waiting for a ride.</p>
		{/if}

		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="sd-list"
			role="toolbar"
			aria-label={view === 'vehicles' ? 'SAG vehicles. f to find a unit, v for requests, Shift+Z to zoom to all' : 'SAG requests and vehicles. v for vehicles, f to find a unit, Shift+Z to zoom to all'}
			aria-orientation="vertical"
			tabindex="-1"
			bind:this={listEl}
			onkeydown={onKey}
			onfocusin={onFocusIn}
		>
			{#if view === 'vehicles'}
				{#each units as g (g.vehicle.checkInId)}
					{@render unitCard(g)}
				{/each}
			{:else}
				{#if unplaced.length > 0}
					<h3 class="sd-group sd-group--flag">⚑ Not on the map <span class="sd-group-n">({unplaced.length})</span></h3>
					{#each unplaced as p (p.request.id)}
						{@render requestCard(p, 'unplaced')}
					{/each}
				{/if}

				{#if waiting.length > 0}
					<h3 class="sd-group">⬒ Waiting <span class="sd-group-n">({waiting.length})</span></h3>
					{#each waiting as p (p.request.id)}
						{@render requestCard(p, 'waiting')}
					{/each}
				{/if}

				{#if inMotion.length > 0}
					<div
						class="sd-item sd-group sd-group--toggle"
						role="button"
						tabindex={tabFor('__motion')}
						aria-expanded={motionOpen}
						aria-label={`In motion, ${inMotion.length} request${inMotion.length === 1 ? '' : 's'}, nobody waiting`}
						onclick={() => (motionOpen = !motionOpen)}
						onkeydown={() => {}}
					>
						<span aria-hidden="true">{motionOpen ? '▾' : '▸'}</span> In motion
						<span class="sd-group-n">({inMotion.length})</span>
					</div>
					{#if motionOpen}
						{#each inMotion as p (p.request.id)}
							{@render requestCard(p, 'motion')}
						{/each}
					{/if}
				{/if}

				{#if noPosition.length > 0}
					<h3 class="sd-group">🚐 No position <span class="sd-group-n">({noPosition.length})</span></h3>
					{#each noPosition as g (g.vehicle.checkInId)}
						<div class="sd-card sd-card--unit">
							<div
								class="sd-item sd-card-hit"
								role="button"
								tabindex={tabFor(`veh:${g.vehicle.checkInId}`)}
								aria-label={`${unitLabel(g.vehicle)}, ${ageState(g.lastHeard, clock) === 'never' ? 'position never set' : 'no position'}, ${g.vehicle.availableSeats} of ${g.vehicle.seats} seats free. Press p to place it on the map.`}
								onclick={() => onPlaceVehicle?.(g.vehicle.checkInId)}
								onkeydown={() => {}}
							>
								<div class="sd-row">
									<span class="sd-unit">{unitLabel(g.vehicle)}</span>
									<span class="sd-sep">·</span>
									<span class="sd-reason sd-reason--inline">voice check-in, position never set</span>
								</div>
								{#if onPlaceVehicle}
									<div class="sd-actions">
										<button class="sd-act sd-act--primary" tabindex="-1" disabled={!$canOperate} onclick={(e) => { e.stopPropagation(); onPlaceVehicle?.(g.vehicle.checkInId); }}>
											Place on map <kbd>p</kbd>
										</button>
									</div>
								{/if}
							</div>
						</div>
					{/each}
				{/if}
			{/if}
		</div>
	{/if}

	{#if onSagFocusPreset}
		<footer class="sd-foot">
			<!-- The label says what the NEXT press does. An operator looking at a
			     map they have quietly stripped of tracks, weather and DF has to
			     be able to see, from here, that they did it. -->
			<button
				class="sd-act"
				class:sd-act--on={$sagFocusPresetOn}
				aria-pressed={$sagFocusPresetOn}
				onclick={onSagFocusPreset}>{$sagFocusPresetOn ? 'Restore my layers' : 'SAG focus'}</button
			>
		</footer>
	{/if}

	<div class="sr-only" aria-live="polite">{announcement}</div>
</section>
{/if}

{#if releaseTarget}
	<ReasonDialog
		title="Release {vname(releaseTarget.leg)} from SAG {releaseTarget.r.sequence}"
		confirmLabel="Release"
		required={false}
		onConfirm={submitRelease}
		onCancel={() => (releaseTarget = null)}
	/>
{/if}

<style>
	/* ---- tablet rail (spec §11) ---- */
	.sd-act--on {
		border-color: var(--color-sag-unit);
		color: var(--color-sag-unit);
	}

	.sd-rail {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-xs);
		width: 44px;
		padding: var(--space-xs) 0;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		box-shadow: var(--shadow-md);
		color: var(--color-text);
		cursor: pointer;
	}

	.sd-rail-title {
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--sag-unit);
	}

	.sd-rail-stub {
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 32px;
		padding: var(--space-2xs) 0;
		border-left: 3px solid var(--sd-tier, var(--color-text-muted));
		border-radius: 2px;
		background: var(--color-bg);
	}

	.sd-rail-stub--flag {
		border-left-color: var(--sag-unit);
	}

	.sd-rail-code {
		font-size: var(--ride-t-label);
		font-weight: 700;
		/* Never the raw tier colour as text: the emergency/medium tokens alias
		   --color-error/--color-info and fail AA on --color-surface. */
		color: var(--color-text);
	}

	.sd-rail-n {
		font-size: var(--ride-t-body);
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
	}

	.sd-rail-quiet,
	.sd-rail-grip {
		font-size: 12px;
		color: var(--color-text-muted);
	}

	.sd-rail-grip {
		margin-top: auto;
	}

	.sd {
		display: flex;
		flex-direction: column;
		min-height: 0;
		max-height: 100%;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		box-shadow: var(--shadow-md);
		overflow: hidden;
	}

	.sd-head {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: 0 var(--space-sm);
		border-bottom: 1px solid var(--color-primary);
		min-height: 44px;
	}

	.sd-title {
		font-size: var(--ride-t-body);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--sag-unit);
	}

	.sd-count {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	/* Header buttons: the design pass's `mini` size (32px), 44px on touch. */
	.sd-head-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-xs);
		min-width: 32px;
		min-height: 32px;
		padding: 0 var(--space-xs);
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-family: inherit;
		font-size: var(--ride-t-label);
		font-weight: 600;
		white-space: nowrap;
		cursor: pointer;
	}

	.sd-fitall,
	.sd-collapse--solo {
		margin-left: auto;
	}

	.sd-fitall:hover,
	.sd-collapse:hover {
		color: var(--color-text);
		border-color: var(--color-accent);
	}

	.sd-find {
		flex: 1;
		min-width: 0;
	}

	.sd-find input {
		width: 100%;
		min-height: 32px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-accent);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: var(--ride-t-body);
	}

	.sd-find-hit {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	/* Requests | Vehicles — a segmented pair under the fixed 44px header band
	   (design pass §3.2: sub-rows go BELOW the band, never inside it). */
	.sd-tabs {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-sm);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.sd-tab {
		flex: 1;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-xs);
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-family: inherit;
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		text-transform: uppercase;
		cursor: pointer;
	}

	.sd-tab[aria-pressed='true'] {
		background: var(--color-primary);
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	/* Count chip (design pass §3.5). */
	.sd-chip {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 18px;
		height: 18px;
		padding: 0 var(--space-xs);
		border-radius: var(--radius-full);
		background: var(--color-bg);
		color: var(--color-text-muted);
		font-size: var(--ride-t-label);
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		letter-spacing: 0;
		line-height: 1;
	}

	.sd-notice,
	.sd-warn,
	.sd-muted {
		margin: var(--space-xs) var(--space-sm);
		font-size: var(--ride-t-label);
		line-height: 1.35;
	}

	.sd-notice {
		color: var(--color-text-muted);
	}

	.sd-muted {
		color: var(--color-text-muted);
	}

	.sd-warn {
		color: var(--color-warning);
		padding: var(--space-xs) var(--space-sm);
		background: var(--color-ride-priority-soft);
		border-radius: var(--radius-sm);
	}

	.sd-warn-head {
		display: block;
		font-weight: 700;
	}

	.sd-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-sm) var(--space-sm);
		overflow-y: auto;
		min-height: 0;
	}

	.sd-group {
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		text-transform: uppercase;
	}

	/* The only group that is never muted: it is the one nothing else renders. */
	.sd-group--flag {
		color: var(--color-warning);
		margin-top: 0;
	}

	.sd-group--toggle {
		cursor: pointer;
		background: none;
		border: none;
		text-align: left;
	}

	.sd-group-n {
		font-weight: 600;
	}

	/* The card is the visual container; its header (`.sd-card-hit`) and each
	   leg row are separate roving items inside it, so no interactive role is
	   ever nested inside another. */
	.sd-card {
		border: 1px solid var(--color-primary);
		border-left: 3px solid var(--sd-tier, var(--color-primary));
		border-radius: var(--radius-sm);
		display: flex;
		flex-direction: column;
		/* A card must never shrink below its own content: inside the phone
		   sheet's constrained column that puts each card's action button on
		   top of the card beneath it. */
		flex-shrink: 0;
	}

	.sd-card--unplaced {
		border-left-style: dashed;
		background: var(--color-ride-priority-soft);
	}

	.sd-card--motion .sd-card-hit {
		color: var(--color-text-muted);
	}

	.sd-card-hit {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-sm);
		cursor: pointer;
		border-radius: var(--radius-sm);
	}

	.sd-card--unit {
		border-left-color: var(--sag-unit);
	}

	.sd-card--focused {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}

	.sd-item:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}

	/* ---- leg rows (S1–S4) ---- */
	.sd-leg {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-xs) var(--space-sm) var(--space-sm);
		border-top: 1px solid var(--color-primary);
		cursor: pointer;
	}

	.sd-leg-line {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		min-width: 0;
		font-size: var(--ride-t-label);
	}

	.sd-leg-line .sd-unit,
	.sd-leg-line .sd-name,
	.sd-leg-line .sd-where {
		font-size: var(--ride-t-label);
	}

	.sd-leg-line .sd-unit {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}

	/* Status pill (design pass §3.5): a WORD, never colour alone. */
	.sd-legstate {
		padding: 0 var(--space-xs);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.sd-legstate[data-legstatus='loaded'] {
		border-color: var(--sag-unit);
	}

	.sd-leg-actions {
		display: flex;
		gap: var(--space-xs);
	}

	.sd-act.sd-go {
		flex: 1;
		min-width: 0;
		display: inline-flex;
		align-items: center;
		justify-content: flex-start;
		gap: var(--space-xs);
		padding: 0 var(--space-sm);
		font-size: var(--ride-t-body);
	}

	.sd-go-veh {
		color: var(--sag-unit);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}

	.sd-go-to {
		white-space: nowrap;
		flex-shrink: 0;
	}

	.sd-legage {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sd-go-text {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* The ✓ after a commit: success border + tint, and inert (COMMIT_LOCK_MS). */
	.sd-act.sd-go--done {
		border-color: var(--color-success);
		background: color-mix(in srgb, var(--color-success) 14%, transparent);
		cursor: default;
	}

	.sd-more {
		min-width: 36px;
		padding: 0;
		font-size: var(--ride-t-body);
		font-weight: 700;
		letter-spacing: 0.1em;
	}

	.sd-legerr {
		font-size: var(--ride-t-label);
		color: var(--color-error-text);
		line-height: 1.35;
	}

	/* ---- inline confirm / more panel ---- */
	.sd-panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		margin: 0 var(--space-sm) var(--space-sm);
		padding: var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-accent);
		border-radius: var(--radius-sm);
		cursor: default;
	}

	.sd-panel-head {
		font-size: var(--ride-t-body);
		font-weight: 600;
		color: var(--color-text);
		line-height: 1.35;
	}

	.sd-panel-sum {
		font-size: var(--ride-t-body);
		color: var(--color-text);
	}

	.sd-panel-note {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		line-height: 1.35;
	}

	.sd-panel-h {
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		line-height: 1.2;
		margin: var(--space-xs) 0 0;
	}

	.sd-panel-h:first-child {
		margin-top: 0;
	}

	.sd-panel-actions {
		display: flex;
		gap: var(--space-xs);
		margin-top: var(--space-xs);
	}

	.sd-panel-actions .sd-act--commit {
		flex: 1;
	}

	.sd-link {
		align-self: flex-start;
		min-height: 32px;
		padding: 0;
		background: none;
		border: none;
		color: var(--color-text);
		font-family: inherit;
		font-size: var(--ride-t-label);
		text-decoration: underline;
		text-underline-offset: 2px;
		cursor: pointer;
	}

	.sd-loadline {
		display: grid;
		grid-template-columns: 1fr;
		gap: var(--space-xs);
	}

	.sd-loadname {
		grid-column: 1 / -1;
		font-size: var(--ride-t-label);
		font-weight: 700;
		color: var(--color-text);
	}

	.sd-select {
		min-width: 0;
		width: 100%;
		min-height: 36px;
		padding: 0 var(--space-xs);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: inherit;
		font-size: var(--ride-t-label);
	}

	.sd-check {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		min-height: 36px;
		font-size: var(--ride-t-label);
		color: var(--color-text);
		cursor: pointer;
	}

	.sd-act.sd-act--commit {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: var(--color-on-accent);
		font-weight: 700;
	}

	.sd-exc {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs);
	}

	.sd-exc .sd-loadname {
		flex: 1 0 100%;
	}

	.sd-act.sd-exc-btn {
		flex: 1;
		padding: 0 var(--space-xs);
		white-space: nowrap;
	}

	.sd-act.sd-menu {
		width: 100%;
		justify-content: flex-start;
		text-align: left;
	}

	.sd-act.sd-menu--danger {
		border-color: var(--color-error);
		color: var(--color-error-text);
	}

	.sd-act.sd-menu--quiet {
		color: var(--color-text-muted);
	}

	/* ---- vehicles view (S6) ---- */
	.sd-unit--big {
		font-size: var(--ride-t-body);
		white-space: nowrap;
	}

	.sd-call {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.sd-pos[data-pos='aging'],
	.sd-pos[data-pos='stale'] {
		color: var(--color-warning);
		font-weight: 600;
	}

	.sd-cap-warn {
		color: var(--color-warning);
		font-weight: 700;
	}

	.sd-pill {
		padding: 0 var(--space-xs);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		white-space: nowrap;
		color: var(--color-text);
	}

	.sd-pill--free {
		border-color: var(--color-success);
	}

	.sd-pill--full {
		border-color: var(--color-warning);
		color: var(--color-warning);
	}

	.sd-row {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: var(--ride-t-body);
		min-width: 0;
	}

	.sd-row--sub {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	/* Tier as TEXT always uses the -text variant: --color-ride-emergency and
	   --color-ride-medium alias --color-error / --color-info and fail AA on
	   --color-surface at normal text size. */
	.sd-code {
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--sd-tier-text, var(--color-text));
		font-size: var(--ride-t-label);
	}

	.sd-name {
		font-weight: 700;
		color: var(--color-text);
		white-space: nowrap;
	}

	.sd-unit {
		font-weight: 700;
		color: var(--sag-unit);
	}

	.sd-sep {
		color: var(--color-text-muted);
	}

	.sd-where,
	.sd-riders {
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sd-age {
		margin-left: auto;
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sd-needs {
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		color: var(--sd-tier-text, var(--color-text-muted));
	}

	.sd-quote {
		font-size: var(--ride-t-label);
		font-style: italic;
		color: var(--color-text);
		line-height: 1.35;
	}

	.sd-reason {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		line-height: 1.35;
	}

	.sd-reason--inline {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sd-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
	}

	.sd-act {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-xs);
		font-family: inherit;
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		cursor: pointer;
	}

	.sd-act--primary {
		border-color: var(--color-accent);
		font-weight: 700;
	}

	.sd-act:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.sd-act kbd {
		margin-left: var(--space-xs);
		opacity: 0.65;
		font-size: 0.9em;
	}

	.sd-foot {
		margin-top: auto;
		padding: var(--space-xs) var(--space-sm);
		border-top: 1px solid var(--color-primary);
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}

	/* Touch sizes: a 1024px tablet is a touch device (spec §11). */
	@media (max-width: 1199px) {
		.sd-card-hit {
			min-height: 44px;
		}

		/* Every control a finger can hit is 44px (--sag-hit) on touch. */
		.sd-act,
		.sd-head-btn,
		.sd-tab,
		.sd-select,
		.sd-check,
		.sd-link,
		.sd-find input {
			min-height: var(--sag-hit, 44px);
		}

		.sd-head-btn,
		.sd-more {
			min-width: var(--sag-hit, 44px);
		}

		/* No keyboard here; key hints are noise (design pass §5.7). */
		.sd-act kbd {
			display: none;
		}
	}
</style>
