<script lang="ts">
	// Ride status strip (WP6, docs/ride-strip-spec.md): 128px two-row band
	// below the map on desktop bike-ride nets. Row A is the course rail
	// (the map's x-axis); Row B is eight zone tiles. The height NEVER grows —
	// it is a contract every phase's zone set must fit inside.
	import CourseRailView from './CourseRailView.svelte';
	import { rideRailModel } from '$lib/stores/courseRoster';
	import { courseStopMiles } from '$lib/stores/courseGeo';
	import { secondClock } from '$lib/stores/clock';
	import RideZone from './RideZone.svelte';
	import PhaseChip from './PhaseChip.svelte';
	import PhaseChangeDialog from './PhaseChangeDialog.svelte';
	import SweepPassedConfirm from './SweepPassedConfirm.svelte';
	import {
		rideZones, rideRail, ridePhase, rideEmergency, rideHasCourse, rideCourseGap, rideAnnouncement, courseState,
		navigateZone, ackEmergency, markSweepPassed, registerRideFlyTo, rideViewportClass, zoneSpanSum
	} from '$lib/stores/ride';
	import { wxIsNcs } from '$lib/stores/wxAlerts';
	import { openAnnotations } from '$lib/stores/ui';
	import { activeCheckIns } from '$lib/stores/netcontrol';

	/** A roster member on the rail -> the map. The rail's pips carry no
	 *  coordinates of their own; the check-in is the one source. */
	function flyToCheckIn(checkInId: string) {
		const ci = $activeCheckIns.find((c) => c.id === checkInId);
		if (ci?.lat != null && ci?.lon != null) onFlyTo(ci.lat, ci.lon, 15);
	}

	/** RECONCILE: the rail's 20px row is a sentence, not a crushed rail (B9). */
	let reconcileLine = $derived.by(() => {
		const c = $courseState;
		const total = (c?.stationsOpen ?? 0) + (c?.stationsClosed ?? 0);
		const sweepAt = $rideRail.edges.sweep?.at;
		const sweepTime = sweepAt
			? new Date(sweepAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
			: null;
		return [
			c?.allStationsClosed ? 'Course clear' : `${c?.stationsOpen ?? 0} stop${c?.stationsOpen === 1 ? '' : 's'} still open`,
			sweepTime ? `sweep last reported ${sweepTime}` : 'sweep not reported',
			total > 0 ? `${c?.stationsClosed ?? 0} of ${total} stops closed` : ''
		]
			.filter(Boolean)
			.join(' · ');
	});

	let {
		isDesktop,
		onFlyTo,
		onFlyToBounds,
		onHeightChange
	}: {
		isDesktop: boolean;
		onFlyTo: (lat: number, lon: number, zoom?: number) => void;
		onFlyToBounds?: (coords: { lat: number; lon: number }[]) => void;
		onHeightChange: (px: number) => void;
	} = $props();

	$effect(() => {
		registerRideFlyTo(onFlyTo, onFlyToBounds ?? null);
		return () => registerRideFlyTo(null, null);
	});

	let wide = $state(true);
	$effect(() => {
		if (!isDesktop) return;
		const mq = window.matchMedia('(min-width: 1200px)');
		wide = mq.matches;
		const handler = (e: MediaQueryListEvent) => (wide = e.matches);
		mq.addEventListener('change', handler);
		return () => mq.removeEventListener('change', handler);
	});

	// The zone SET (stores/ride.ts's rideZones) is a function of this same
	// >=1200px split, not just the strip's own height/CSS — see docs/
	// ride-strip-spec.md §9 and finding 12 (tablet zone merging).
	$effect(() => {
		rideViewportClass.set(wide ? 'desktop' : 'tablet');
	});

	let stripH = $derived(!$rideHasCourse ? 66 : $ridePhase?.phase === 'reconcile' ? 90 : wide ? 128 : 140);

	// Explicit column count derived from the RESOLVED zone set (never
	// hardcoded, never implicit) — the grid always has exactly as many
	// equal-width columns as the zone set's total span, so a phase/viewport
	// combination can never be asked to squeeze into fewer columns than it
	// needs. `grid-auto-flow: column` (below) is kept as a second, redundant
	// guard: even if this number were ever wrong, column-flow can only ever
	// add MORE columns to the same row, never wrap to a hidden second row —
	// which is the exact failure that once ate the EMERGENCY Acknowledge
	// button.
	let zoneCols = $derived(Math.max(1, zoneSpanSum($rideZones)));

	let focusIdx = $state(0);
	let showSweepConfirm = $state(false);
	let phaseDialogOpen = $state(false);
	let zonesContainer = $state<HTMLElement | undefined>();

	function focusZone(idx: number) {
		const hits = zonesContainer?.querySelectorAll<HTMLElement>('.ride-zone-hit');
		hits?.[idx]?.focus();
	}

	// The zone SET changes with phase and with an EMERGENCY absorbing SAG+WX.
	// A roving tabindex whose index outruns the list leaves NO zone at
	// tabindex 0, and the whole strip silently drops out of the tab order —
	// the exact failure the single-tab-stop pattern exists to prevent.
	$effect(() => {
		const n = $rideZones.length;
		if (n > 0 && focusIdx > n - 1) focusIdx = n - 1;
	});

	// Clicking or shift-tabbing into a zone has to move the rove with it,
	// or the next arrow press jumps back to wherever the keyboard last was.
	function onZoneFocusIn(e: FocusEvent) {
		const hits = zonesContainer?.querySelectorAll<HTMLElement>('.ride-zone-hit');
		if (!hits) return;
		const idx = Array.prototype.indexOf.call(hits, (e.target as HTMLElement)?.closest('.ride-zone-hit'));
		if (idx >= 0) focusIdx = idx;
	}

	// Publish --ride-strip-h exactly the way BottomSheet publishes --sheet-peek.
	$effect(() => {
		const root = document.documentElement;
		const prev = root.style.getPropertyValue('--ride-strip-h');
		root.style.setProperty('--ride-strip-h', `${stripH}px`);
		onHeightChange(stripH);
		return () => {
			if (prev) root.style.setProperty('--ride-strip-h', prev);
			else root.style.removeProperty('--ride-strip-h');
			onHeightChange(0);
		};
	});

	function onToolbarKey(e: KeyboardEvent) {
		const n = $rideZones.length;
		if (n === 0) return;
		if (e.key === 'ArrowRight') {
			focusIdx = (focusIdx + 1) % n;
			focusZone(focusIdx);
			e.preventDefault();
		} else if (e.key === 'ArrowLeft') {
			focusIdx = (focusIdx - 1 + n) % n;
			focusZone(focusIdx);
			e.preventDefault();
		} else if (e.key === 'Home') {
			focusIdx = 0;
			focusZone(focusIdx);
			e.preventDefault();
		} else if (e.key === 'End') {
			focusIdx = n - 1;
			focusZone(focusIdx);
			e.preventDefault();
		}
	}
</script>

<section
	class="ride-strip"
	class:ride-strip--emergency={$rideEmergency != null}
	class:ride-strip--wide={wide}
	class:ride-strip--nocourse={!$rideHasCourse}
	class:ride-strip--summary={$rideHasCourse && $ridePhase?.phase === 'reconcile'}
	aria-label="Ride status"
	style="height: {stripH}px"
>
	{#if !$rideHasCourse}
		<!-- Two causes, two different remedies. Telling an operator who HAS
		     imported the GPX to "import GPX" sent them round the loop again;
		     what is actually missing is a sequence number on the stops. -->
		<div class="ride-nocourse">
			{#if $rideCourseGap === 'no-sequenced-stops'}
				<span>Course line loaded, but no stops are numbered — give each rest stop a Seq # to enable ride mode</span>
				<button class="ride-nocourse-btn" onclick={() => openAnnotations()}>Number stops</button>
			{:else}
				<span>No course loaded — import GPX to enable ride mode</span>
				<button class="ride-nocourse-btn" onclick={() => openAnnotations()}>Import GPX</button>
			{/if}
		</div>
	{:else}
		<div class="ride-rail">
			{#if $ridePhase?.phase === 'reconcile'}
				<p class="ride-reconcile-line">{reconcileLine}</p>
			{:else}
				<CourseRailView
					model={$rideRailModel}
					now={$secondClock}
					density="strip"
					onStopActivate={(cpId) => navigateZone('stop', cpId)}
					onRosterActivate={flyToCheckIn}
					onFlyTo={(lat, lon) => onFlyTo(lat, lon, 14)}
					onSweepPassed={() => (showSweepConfirm = true)}
					onRosterList={() => navigateZone('checkedIn')}
				/>
			{/if}
		</div>

		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="ride-zones"
			class:ride-zones--wide={wide}
			role="toolbar"
			tabindex="-1"
			aria-label="Ride status zones"
			bind:this={zonesContainer}
			style:--ride-zone-cols={zoneCols}
			onkeydown={onToolbarKey}
			onfocusin={onZoneFocusIn}
		>
			{#each $rideZones as z, i (z.id)}
				<RideZone
					id={z.id}
					label={z.label}
					lines={z.lines}
					span={z.span}
					tone={z.tone}
					borderVar={z.borderVar}
					ariaSentence={z.aria}
					focused={i === focusIdx}
					onActivate={() => navigateZone(z.id)}
				>
					{#if z.id === 'net'}
						<PhaseChip state={$ridePhase} canSet={$wxIsNcs} onOpen={() => (phaseDialogOpen = true)} />
					{:else if z.id === 'traffic' && $rideEmergency}
						<button
							class="ride-ack"
							onclick={(e) => {
								e.stopPropagation();
								if ($rideEmergency) ackEmergency($rideEmergency.id);
							}}
						>
							Acknowledge <kbd>a</kbd>
						</button>
					{/if}
				</RideZone>
			{/each}
		</div>
	{/if}

	<div class="sr-only" aria-live="polite">{$rideAnnouncement}</div>

	{#if showSweepConfirm}
		<SweepPassedConfirm
			stations={$rideRail.stations}
			defaultCheckpointId={$rideRail.sweepNextStationId}
			sweepLabel={$rideRailModel.sweepLabel}
			stopMiles={$courseStopMiles}
			onConfirm={async (cpId) => {
				await markSweepPassed(cpId);
				showSweepConfirm = false;
			}}
			onCancel={() => (showSweepConfirm = false)}
		/>
	{/if}

	{#if phaseDialogOpen}
		<PhaseChangeDialog phaseState={$ridePhase} onClose={() => (phaseDialogOpen = false)} />
	{/if}
</section>

<style>
	.ride-strip {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: var(--z-toolbar);
		background: var(--color-surface);
		border-top: 1px solid var(--color-primary);
		display: grid;
		grid-template-rows: 60px 1fr;
		box-shadow: var(--shadow-md);
		overflow: hidden;
	}

	/* The row template has to track `stripH`, or `overflow: hidden` eats the
	   difference. With no course there is one child and 66px, so a 60px first
	   row left the message boxed into 60 and the button clipped; in RECONCILE
	   the rail collapses to a 20px summary line and the strip to 90px, and a
	   60px first row left the five closeout tiles 30px of the 66px they need. */
	.ride-strip--nocourse {
		grid-template-rows: 1fr;
	}

	.ride-strip--summary {
		grid-template-rows: 20px 1fr;
	}

	.ride-strip--emergency {
		border-top: 4px solid var(--color-ride-emergency);
		animation: ride-em-pulse 2s ease-in-out 3;
	}

	@keyframes ride-em-pulse {
		0%,
		100% {
			border-top-color: var(--color-ride-emergency);
		}
		50% {
			border-top-color: var(--color-ride-emergency-soft);
		}
	}

	.ride-nocourse {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-md);
		height: 100%;
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
	}

	.ride-nocourse-btn {
		min-height: 36px;
		padding: 0 var(--space-md);
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-on-accent);
		font-weight: 700;
		cursor: pointer;
	}

	.ride-rail {
		position: relative;
		border-bottom: 1px solid var(--color-primary);
		padding: 0 var(--space-md);
		overflow: hidden;
	}

	.ride-reconcile-line {
		margin: 0;
		font-size: var(--ride-t-label);
		font-weight: 700;
		letter-spacing: var(--ride-label-tracking);
		text-transform: uppercase;
		color: var(--color-text-muted);
		line-height: 20px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	/* The zone SET is phase-dependent (5 zones in PRE-START, 8 in MID-RIDE,
	   9 column-units in CLOSING where two zones span 2) and WX is
	   conditional, so a fixed-count template silently pushes the tail zones
	   onto an implicit second row that `overflow: hidden` then eats — which
	   is how SHIFT vanished in CLOSING and TRAFFIC (with its Acknowledge
	   button) vanished in EMERGENCY. Auto-flow columns keep every zone on
	   the one row at any count; `span` still buys a zone 2x/3x the share,
	   which is what carried the spec's per-zone width ranking anyway. */
	.ride-zones {
		display: grid;
		/* Explicit template derived from the resolved zone set's own span sum
		   (--ride-zone-cols, set by RideStrip.svelte) rather than a hardcoded
		   count — this is what makes tablet's six merged zones land as six
		   genuinely equal columns instead of eight narrow ones. grid-auto-flow:
		   column + grid-auto-columns stay on as a second guard: if a future
		   zone set ever produced more span-units than --ride-zone-cols
		   accounts for, column-flow can only add MORE columns to this one row
		   — it can never wrap into an implicit second row for overflow:hidden
		   to silently eat, which is exactly how the EMERGENCY Acknowledge
		   button once vanished. */
		grid-template-columns: repeat(var(--ride-zone-cols, 1), minmax(0, 1fr));
		grid-auto-flow: column;
		grid-auto-columns: minmax(0, 1fr);
		gap: var(--space-xs);
		padding: 3px var(--space-md);
		overflow: hidden;
	}

	.ride-ack {
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: var(--color-ride-emergency);
		color: var(--color-on-accent);
		border: none;
		border-radius: var(--radius-sm);
		font-weight: 800;
		letter-spacing: 0.04em;
		cursor: pointer;
	}

	.ride-ack kbd {
		font-size: var(--ride-t-label);
		opacity: 0.8;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}
</style>
