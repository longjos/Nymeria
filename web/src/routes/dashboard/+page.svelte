<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { api, loadSavedToken, setAuthToken } from '$lib/api';
	import { WSClient } from '$lib/ws';
	import {
		initSession, isLoggedIn, isPending, isDenied, isApproved,
		currentUser, handleSessionEvent
	} from '$lib/stores/session';
	import type {
		Net, NetCheckIn, NetMission, NetEvent, Annotation,
		CheckpointWithPassages, CheckpointPassage, ProgressElement
	} from '$lib/types';
	import DashboardLogin from '$lib/components/dashboard/DashboardLogin.svelte';
	import AgencyHeader from '$lib/components/dashboard/AgencyHeader.svelte';
	import AgencyMap from '$lib/components/dashboard/AgencyMap.svelte';
	import ResourceSummary from '$lib/components/dashboard/ResourceSummary.svelte';
	import IncidentFeed from '$lib/components/dashboard/IncidentFeed.svelte';
	import EventProgress from '$lib/components/dashboard/EventProgress.svelte';

	// --- Local state (not shared with main app stores) ---
	let sessionReady = $state(false);
	let net = $state<Net | null>(null);
	let checkIns = $state<NetCheckIn[]>([]);
	let missions = $state<NetMission[]>([]);
	let dashAnnotations = $state<Annotation[]>([]);
	let checkpoints = $state<CheckpointWithPassages[]>([]);
	let lastUpdated = $state(new Date());
	let wsConnected = $state(false);
	let presentationMode = $state(false);
	let loadError = $state('');

	let ws: WSClient | null = null;

	// --- Derived state ---
	let activeOps = $derived(checkIns.filter(ci => ci.status !== 'released'));
	let opsWithPosition = $derived(activeOps.filter(ci => ci.lat != null && ci.lon != null));
	let activeMissions = $derived(
		missions.filter(m => m.status !== 'complete')
			.sort((a, b) => {
				const prio: Record<string, number> = { emergency: 0, priority: 1, welfare: 2, routine: 3 };
				return (prio[a.priority] ?? 3) - (prio[b.priority] ?? 3);
			})
	);

	let assignmentLines = $derived.by(() => {
		const missionMap = new Map(missions.map(m => [m.id, m]));
		const lines: { operator: NetCheckIn; mission: NetMission }[] = [];
		for (const ci of activeOps) {
			if (!ci.missionIds?.length || ci.lat == null || ci.lon == null) continue;
			for (const mid of ci.missionIds) {
				const mission = missionMap.get(mid);
				if (mission && mission.lat != null && mission.lon != null) {
					lines.push({ operator: ci, mission });
				}
			}
		}
		return lines;
	});

	let opsViewObj = $derived.by(() => {
		if (!net) return null;
		if (net.opsViewLat != null && net.opsViewLon != null && net.opsViewZoom != null) {
			return { lat: net.opsViewLat, lon: net.opsViewLon, zoom: net.opsViewZoom };
		}
		return null;
	});

	let orderedCheckpoints = $derived(
		[...checkpoints].sort((a, b) => a.meta.sequenceNumber - b.meta.sequenceNumber)
	);

	let progressElements = $derived.by((): ProgressElement[] => {
		const seqMap = new Map<string, number>();
		for (const cp of checkpoints) {
			seqMap.set(cp.meta.annotationId, cp.meta.sequenceNumber);
		}

		const elemMap = new Map<string, ProgressElement>();
		for (const cp of checkpoints) {
			for (const p of cp.passages) {
				const seq = seqMap.get(p.checkpointId);
				if (seq == null) continue;
				const existing = elemMap.get(p.label);
				if (!existing || seq > existing.lastCheckpointSeq) {
					elemMap.set(p.label, {
						label: p.label,
						lastCheckpointId: p.checkpointId,
						lastCheckpointSeq: seq,
						lastPassageTime: p.passageTime,
					});
				}
			}
		}

		return [...elemMap.values()].sort((a, b) => b.lastCheckpointSeq - a.lastCheckpointSeq);
	});

	// --- Lifecycle ---
	onMount(async () => {
		await initSession();
		sessionReady = true;

		return () => {
			ws?.disconnect();
			ws = null;
		};
	});

	// When approved, load data and connect WS
	$effect(() => {
		if ($isApproved && !ws) {
			loadDashboardData();
			connectDashboardWS();
		}
	});

	async function loadDashboardData() {
		try {
			// Determine net ID: from URL param or auto-detect
			const urlNetId = $page.url.searchParams.get('net');
			let netId = urlNetId;

			if (!netId) {
				const nets = await api.nets();
				const open = nets.find(n => n.status === 'open');
				if (!open) {
					net = null;
					return;
				}
				netId = open.id;
			}

			// Fetch net data
			const data = await api.net(netId);
			net = data.net;
			checkIns = data.checkIns || [];
			missions = data.missions || [];
			dashAnnotations = (data.annotations || []).filter(a => a.netId === netId);
			lastUpdated = new Date();

			// Fetch checkpoints
			try {
				checkpoints = await api.getCheckpoints(netId);
			} catch {
				checkpoints = [];
			}
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load data';
		}
	}

	function connectDashboardWS() {
		if (!$currentUser) return;
		ws = new WSClient();
		ws.connect(undefined, $currentUser.token);

		// Session events
		ws.on('access_approved', handleSessionEvent);
		ws.on('access_denied', handleSessionEvent);

		// Net events
		ws.on('net_updated', (msg) => {
			const n = msg.data as Net;
			if (!n || !net || n.id !== net.id) return;
			net = n;
			lastUpdated = new Date();
		});

		ws.on('checkin_created', (msg) => {
			const ci = msg.data as NetCheckIn;
			if (!ci) return;
			checkIns = [...checkIns, ci];
			lastUpdated = new Date();
		});

		ws.on('checkin_updated', (msg) => {
			const ci = msg.data as NetCheckIn;
			if (!ci) return;
			checkIns = checkIns.map(c => c.id === ci.id ? ci : c);
			lastUpdated = new Date();
		});

		ws.on('mission_created', (msg) => {
			const m = msg.data as NetMission;
			if (!m) return;
			missions = [...missions, m];
			lastUpdated = new Date();
		});

		ws.on('mission_updated', (msg) => {
			const m = msg.data as NetMission;
			if (!m) return;
			missions = missions.map(existing => existing.id === m.id ? m : existing);
			lastUpdated = new Date();
		});

		ws.on('checkpoint_passage', (msg) => {
			const passage = msg.data as CheckpointPassage;
			if (!passage) return;
			checkpoints = checkpoints.map(cp => {
				if (cp.meta.annotationId !== passage.checkpointId) return cp;
				return {
					...cp,
					passages: [...cp.passages, passage],
					passageCount: cp.passageCount + 1,
					latestPassage: passage.passageTime,
				};
			});
			lastUpdated = new Date();
		});

		ws.on('checkpoint_meta_updated', (msg) => {
			const meta = msg.data as any;
			if (!meta) return;
			checkpoints = checkpoints.map(cp => {
				if (cp.meta.annotationId !== meta.annotationId) return cp;
				return { ...cp, meta: { ...cp.meta, ...meta } };
			});
			lastUpdated = new Date();
		});

		ws.on('annotation_created', (msg) => {
			const ann = msg.data as Annotation;
			if (!ann || ann.netId !== net?.id) return;
			dashAnnotations = [...dashAnnotations, ann];
			lastUpdated = new Date();
		});

		ws.on('annotation_updated', (msg) => {
			const ann = msg.data as Annotation;
			if (!ann) return;
			if (ann.netId === net?.id) {
				const exists = dashAnnotations.some(a => a.id === ann.id);
				dashAnnotations = exists
					? dashAnnotations.map(a => a.id === ann.id ? ann : a)
					: [...dashAnnotations, ann];
			} else {
				dashAnnotations = dashAnnotations.filter(a => a.id !== ann.id);
			}
			lastUpdated = new Date();
		});

		// Track connection state
		wsConnected = true;
	}

	let ncsCheckIn = $derived(
		net ? checkIns.find(ci => ci.callsign === net!.ncsCallsign) : undefined
	);
</script>

<svelte:head>
	<title>{net ? `${net.name} — Dashboard` : 'Operations Dashboard'} | Nymeria</title>
</svelte:head>

<div class="dashboard" class:presentation={presentationMode}>
	{#if !sessionReady}
		<div class="dashboard-loading">
			<div class="spinner"></div>
			<p>Loading...</p>
		</div>
	{:else if !$isLoggedIn || $isPending || $isDenied}
		<DashboardLogin />
	{:else if loadError}
		<div class="dashboard-empty">
			<p class="error-text">Error: {loadError}</p>
			<button class="retry-btn" onclick={loadDashboardData}>Retry</button>
		</div>
	{:else if !net}
		<div class="dashboard-empty">
			<div class="empty-icon">
				<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
					<circle cx="12" cy="12" r="10"/>
					<path d="M12 8v4M12 16h.01"/>
				</svg>
			</div>
			<h2>No Active Net</h2>
			<p>There is no active operations net at this time.</p>
		</div>
	{:else}
		<AgencyHeader
			{net}
			{ncsCheckIn}
			{lastUpdated}
			connected={wsConnected}
			{presentationMode}
			onTogglePresentation={() => { presentationMode = !presentationMode; }}
		/>

		<div class="dashboard-content">
			<div class="map-area">
				<AgencyMap
					operators={opsWithPosition}
					missions={activeMissions.filter(m => m.lat != null && m.lon != null)}
					annotations={dashAnnotations}
					assignments={assignmentLines}
					opsView={opsViewObj}
				/>
			</div>

			<div class="sidebar-area">
				<ResourceSummary checkIns={checkIns} {presentationMode} />

				<IncidentFeed missions={missions} checkIns={checkIns} {presentationMode} />

				{#if orderedCheckpoints.length > 0}
					<EventProgress
						checkpoints={orderedCheckpoints}
						elements={progressElements}
						{presentationMode}
					/>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	:global(body) {
		/* Override layout's overflow:hidden for dashboard scrolling on mobile */
		overflow: auto !important;
	}

	.dashboard {
		display: flex;
		flex-direction: column;
		height: 100vh;
		height: 100dvh;
		background: var(--color-bg);
	}

	.dashboard.presentation {
		font-size: 1.15em;
	}

	.dashboard-loading {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-md);
		color: var(--color-text-muted);
	}

	.spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--color-primary);
		border-top-color: var(--color-accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	.dashboard-empty {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-md);
		color: var(--color-text-muted);
		padding: var(--space-xl);
	}

	.empty-icon {
		opacity: 0.4;
	}

	.dashboard-empty h2 {
		font-size: 1.2rem;
		color: var(--color-text);
	}

	.dashboard-empty p {
		font-size: 0.9rem;
	}

	.error-text {
		color: var(--color-error);
	}

	.retry-btn {
		padding: 8px 20px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text);
		font-size: 0.85rem;
		cursor: pointer;
	}

	.retry-btn:hover {
		border-color: var(--color-accent);
	}

	.dashboard-content {
		flex: 1;
		display: grid;
		grid-template-columns: 3fr 2fr;
		min-height: 0;
	}

	.map-area {
		min-height: 0;
		position: relative;
	}

	.sidebar-area {
		overflow-y: auto;
		border-left: 1px solid var(--color-primary);
	}

	/* Presentation mode: more room for map */
	.presentation .dashboard-content {
		grid-template-rows: 50vh 1fr;
		grid-template-columns: 1fr;
	}

	.presentation .sidebar-area {
		border-left: none;
		border-top: 1px solid var(--color-primary);
		display: flex;
		gap: var(--space-md);
		overflow-x: auto;
		padding: var(--space-sm);
	}

	.presentation .sidebar-area > :global(*) {
		min-width: 300px;
		flex: 1;
	}

	/* Mobile: stacked layout */
	@media (max-width: 768px) {
		.dashboard {
			height: auto;
			min-height: 100vh;
			min-height: 100dvh;
		}

		.dashboard-content {
			grid-template-columns: 1fr;
			grid-template-rows: 40vh auto;
		}

		.sidebar-area {
			border-left: none;
			border-top: 1px solid var(--color-primary);
			overflow-y: visible;
		}
	}
</style>
