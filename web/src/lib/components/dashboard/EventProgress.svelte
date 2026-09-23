<script lang="ts">
	/**
	 * The dashboard's course rail. The dashboard keeps its own data — its own
	 * socket, no shared session with the main app — so it builds the rail's
	 * model itself, with the same pure buildRailModel the main app uses. That
	 * is what makes this rail, the ride strip and the SituationBoard show one
	 * course the same way: to scale, with roster members on it, and LEAD and
	 * SWEEP from reported sources only.
	 */
	import type { Annotation, CheckpointWithPassages, CourseState, NetCheckIn, RidePhaseID, WxAlert } from '$lib/types';
	import { buildRouteIndex, parseLineString, type RouteIndex } from '$lib/routeDistance';
	import { buildRailModel } from '$lib/railModel';
	import type { RosterMemory } from '$lib/rosterCourse';
	import CourseRailView from '../CourseRailView.svelte';

	let {
		checkpoints,
		annotations,
		checkIns,
		courseState = null,
		phase = null,
		now,
		presentationMode = false,
		wxAlerts = []
	}: {
		checkpoints: CheckpointWithPassages[];
		/** The net's annotations — the course line and each stop's LIVE state. */
		annotations: Annotation[];
		checkIns: NetCheckIn[];
		/** Bike-ride nets only: closures, shutoffs, sweep reports. */
		courseState?: CourseState | null;
		phase?: RidePhaseID | null;
		now: number;
		presentationMode?: boolean;
		/** Active IN alerts — the dashboard keeps its own snapshot (no shared session with the main app). */
		wxAlerts?: WxAlert[];
	} = $props();

	// The course line, parsed once per revision.
	let routeCache: { key: string; idx: RouteIndex | null } = { key: '', idx: null };
	let routeIndex = $derived.by(() => {
		const route = annotations.find((a) => a.category === 'route');
		if (!route) return null;
		const key = `${route.id}:${route.updatedAt ?? ''}`;
		if (routeCache.key !== key) {
			let idx: RouteIndex | null = null;
			try {
				const coords = parseLineString(route.geometry);
				idx = coords ? buildRouteIndex(coords) : null;
			} catch {
				idx = null;
			}
			routeCache = { key, idx };
		}
		return routeCache.idx;
	});

	/** Continuity memory for the roster projection, per dashboard session. */
	const memory: RosterMemory = new Map();

	let model = $derived(
		buildRailModel({
			routeIndex,
			checkpoints,
			liveAnnotations: new Map(annotations.map((a) => [a.id, a])),
			courseState,
			checkIns: checkIns
				.filter((ci) => ci.status !== 'released')
				.map((ci) => ({
					id: ci.id,
					callsign: ci.callsign,
					tacticalCall: ci.tacticalCall,
					category: ci.category ?? 'general',
					lat: ci.lat,
					lon: ci.lon,
					lastHeard: ci.lastHeard,
					source: ci.source
				})),
			rosterMemory: memory,
			wxAlerts,
			phase,
			nowMs: now
		})
	);
</script>

{#if checkpoints.length > 0}
	<section class="event-progress" class:presentation={presentationMode}>
		<h2 class="section-title">Event progress</h2>
		<CourseRailView {model} {now} density="panel" presentation={presentationMode} />
	</section>
{/if}

<style>
	.event-progress {
		padding: var(--space-md);
	}

	/* The ride-dashboard design pass's section-header recipe. */
	.section-title {
		margin: 0 0 var(--space-sm);
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
	}

	.presentation .section-title {
		font-size: var(--ride-t-body);
	}
</style>
