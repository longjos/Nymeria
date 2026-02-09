<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import Map from '$lib/components/Map.svelte';
	import Toolbar from '$lib/components/Toolbar.svelte';
	import ActivityRail from '$lib/components/ActivityRail.svelte';
	import SidePanel from '$lib/components/SidePanel.svelte';
	import BottomSheet from '$lib/components/BottomSheet.svelte';
	import StationList from '$lib/components/StationList.svelte';
	import StationDetail from '$lib/components/StationDetail.svelte';
	import ConvoList from '$lib/components/ConvoList.svelte';
	import TransportPanel from '$lib/components/TransportPanel.svelte';
	import ActivityPanel from '$lib/components/ActivityPanel.svelte';
	import AnnotationPanel from '$lib/components/AnnotationPanel.svelte';
	import NetControlPanel from '$lib/components/NetControlPanel.svelte';
	import BulletinPanel from '$lib/components/BulletinPanel.svelte';
	import ICS309Panel from '$lib/components/ICS309Panel.svelte';
	import ConnectionStatus from '$lib/components/ConnectionStatus.svelte';
	import SearchOverlay from '$lib/components/SearchOverlay.svelte';
	import LoginOverlay from '$lib/components/LoginOverlay.svelte';
	import { stations, stationList, initStationStore } from '$lib/stores/stations';
	import { initMessageStore, conversationList } from '$lib/stores/messages';
	import { initTransportStore } from '$lib/stores/transports';
	import { annotationList, initAnnotationStore } from '$lib/stores/annotations';
	import {
		initNetControlStore,
		activeNet, operatorsWithPosition, missionsWithPosition, assignmentLines,
		opsView
	} from '$lib/stores/netcontrol';
	import { initTacticalStore } from '$lib/stores/tactical';
	import { initBulletinStore } from '$lib/stores/bulletins';
	import { isLoggedIn, initSession } from '$lib/stores/session';
	import {
		selectedStation, panelMode, detailTab, searchOpen, sheetState,
		selectStation, closePanel, openStationList, openMessages, openConversation, openTransports, openActivity, openAnnotations, openNetControl, openBulletins, openICS309,
		togglePanel
	} from '$lib/stores/ui';
	import type { SheetState, DetailTab, PanelMode } from '$lib/stores/ui';
	import type { Annotation } from '$lib/types';

	let isDesktop = $state(true);
	let flyToTarget = $state<{ lat: number; lon: number; zoom?: number } | null>(null);
	let sessionReady = $state(false);
	let drawingMode = $state<'point' | 'line' | 'area' | null>(null);
	let previewGeometry = $state<string | null>(null);
	let previewColor = $state('#e63946');
	let annotationPanelRef = $state<AnnotationPanel>();
	let mapRef = $state<Map>();
	let editingAnnotationId = $state<string | null>(null);

	let stationsWithPosition = $derived(
		$stationList.filter((s) => s.position)
	);

	let totalUnread = $derived(
		$conversationList.reduce((sum, c) => sum + c.unreadCount, 0)
	);

	let panelIsOpen = $derived($panelMode !== 'closed');

	// Watch login state — init data stores when user logs in
	$effect(() => {
		if ($isLoggedIn) {
			initStationStore();
			initMessageStore();
			initTransportStore();
			initAnnotationStore();
			initNetControlStore();
			initTacticalStore();
			initBulletinStore();
		}
	});

	onMount(async () => {
		await initSession();
		sessionReady = true;

		if (browser) {
			const mq = window.matchMedia('(min-width: 769px)');
			isDesktop = mq.matches;
			const handler = (e: MediaQueryListEvent) => { isDesktop = e.matches; };
			mq.addEventListener('change', handler);
			return () => mq.removeEventListener('change', handler);
		}
	});

	function handleStationClick(key: string) {
		selectStation(key);
		// Fly to the station
		const st = $stations.get(key);
		if (st?.position) {
			flyToTarget = { lat: st.position.lat, lon: st.position.lon };
		}
	}

	function handleSearchSelect(key: string) {
		selectStation(key);
		searchOpen.set(false);
		const st = $stations.get(key);
		if (st?.position) {
			flyToTarget = { lat: st.position.lat, lon: st.position.lon };
		}
	}

	function handleFlyTo(lat: number, lon: number) {
		flyToTarget = { lat, lon };
	}

	function handleTabChange(tab: DetailTab) {
		detailTab.set(tab);
	}

	function handleSheetStateChange(s: SheetState) {
		sheetState.set(s);
	}

	function handleConvoSelect(callsign: string) {
		openConversation(callsign);
	}

	function handleAnnotationClick(id: string) {
		openAnnotations();
	}

	function handleNetOperatorClick(_checkInId: string) {
		openNetControl();
	}

	function handleNetMissionClick(_missionId: string) {
		openNetControl();
	}

	function handleNetFlyTo(lat: number, lon: number) {
		flyToTarget = { lat, lon, zoom: 15 };
	}

	function handleSetOpsView() {
		const vp = mapRef?.getViewport();
		if (vp) {
			opsView.set(vp);
		}
	}

	function handleGoToOpsView() {
		const v = $opsView;
		if (v) {
			flyToTarget = { lat: v.lat, lon: v.lon, zoom: v.zoom };
		}
	}

	function handleFlyToAnnotation(ann: Annotation) {
		try {
			const geom = JSON.parse(ann.geometry);
			if (geom.type === 'Point') {
				flyToTarget = { lat: geom.coordinates[1], lon: geom.coordinates[0], zoom: 15 };
			} else if (geom.type === 'LineString') {
				const mid = Math.floor(geom.coordinates.length / 2);
				flyToTarget = { lat: geom.coordinates[mid][1], lon: geom.coordinates[mid][0], zoom: 14 };
			} else if (geom.type === 'Polygon') {
				const coords = geom.coordinates[0];
				let latSum = 0, lonSum = 0;
				for (const c of coords) { latSum += c[1]; lonSum += c[0]; }
				flyToTarget = { lat: latSum / coords.length, lon: lonSum / coords.length, zoom: 14 };
			}
		} catch { /* skip */ }
	}

	function handleStartDraw(mode: 'point' | 'line' | 'area') {
		drawingMode = mode;
	}

	function handleDrawComplete(geometry: string) {
		drawingMode = null;
		annotationPanelRef?.setGeometry(geometry);
	}

	function handlePreviewChange(geometry: string | null, color: string) {
		previewGeometry = geometry;
		previewColor = color;
	}

	function handleStartEdit(id: string) {
		editingAnnotationId = id;
		// Fly to the annotation
		const ann = $annotationList.find((a) => a.id === id);
		if (ann) handleFlyToAnnotation(ann);
	}

	function handleStopEdit() {
		editingAnnotationId = null;
	}

	function handleGeometryEdit(geometry: string) {
		annotationPanelRef?.setEditGeometry(geometry);
	}

	function handlePreviewGeometryChange(geometry: string) {
		previewGeometry = geometry;
	}

	function handleRailToggle(mode: PanelMode) {
		togglePanel(mode);
	}
</script>

<svelte:head>
	<title>Nymeria - APRS Client</title>
</svelte:head>

<div class="app-container">
	<!-- Login gate -->
	{#if sessionReady && !$isLoggedIn}
		<LoginOverlay />
	{/if}

	<!-- Full-screen map -->
	<div class="map-layer">
		<Map
			bind:this={mapRef}
			stations={stationsWithPosition}
			annotations={$annotationList}
			selectedCallsign={$selectedStation ?? ''}
			onStationClick={handleStationClick}
			onAnnotationClick={handleAnnotationClick}
			{flyToTarget}
			panelOpen={panelIsOpen}
			{drawingMode}
			onDrawComplete={handleDrawComplete}
			{previewGeometry}
			{previewColor}
			{editingAnnotationId}
			onGeometryEdit={handleGeometryEdit}
			onPreviewGeometryChange={handlePreviewGeometryChange}
			netOperators={$operatorsWithPosition}
			netMissions={$missionsWithPosition}
			netAssignmentLines={$assignmentLines}
			activeNetId={$activeNet?.id ?? null}
			onNetOperatorClick={handleNetOperatorClick}
			onNetMissionClick={handleNetMissionClick}
		/>
	</div>

	<!-- Desktop: Activity Rail (right edge) -->
	{#if isDesktop}
		<ActivityRail
			panelMode={$panelMode}
			unreadCount={totalUnread}
			onToggle={handleRailToggle}
			onSelectStation={handleSearchSelect}
		/>
	{/if}

	<!-- Mobile: Toolbar (FABs) -->
	<Toolbar
		unreadCount={totalUnread}
		onSearchOpen={() => searchOpen.set(true)}
		onMessagesOpen={openMessages}
		onBulletinsOpen={openBulletins}
		onTransportsOpen={openTransports}
		onAnnotationsOpen={openAnnotations}
		onNetControlOpen={openNetControl}
	/>

	<!-- Desktop: Side Panel -->
	{#if isDesktop}
		<SidePanel
			open={panelIsOpen}
			onClose={closePanel}
			onTransitionEnd={() => {}}
		>
			{#if $panelMode === 'stations'}
				<StationList
					onSelect={handleStationClick}
					selectedKey={$selectedStation}
				/>
			{:else if $panelMode === 'detail' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
				/>
			{:else if $panelMode === 'messages'}
				<ConvoList
					onSelectConvo={handleConvoSelect}
				/>
			{:else if $panelMode === 'convo' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
				/>
			{:else if $panelMode === 'transports'}
				<TransportPanel />
			{:else if $panelMode === 'activity'}
				<ActivityPanel />
			{:else if $panelMode === 'annotations'}
				<AnnotationPanel
					bind:this={annotationPanelRef}
					onFlyToAnnotation={handleFlyToAnnotation}
					onStartDraw={handleStartDraw}
					onPreviewChange={handlePreviewChange}
					onStartEdit={handleStartEdit}
					onStopEdit={handleStopEdit}
				/>
			{:else if $panelMode === 'bulletins'}
				<BulletinPanel />
			{:else if $panelMode === 'netcontrol'}
				<NetControlPanel
					onFlyTo={handleNetFlyTo}
					onSetOpsView={handleSetOpsView}
					onGoToOpsView={handleGoToOpsView}
				/>
			{:else if $panelMode === 'ics309'}
				<ICS309Panel />
			{/if}
		</SidePanel>
	{/if}

	<!-- Mobile: Bottom Sheet -->
	{#if !isDesktop}
		<BottomSheet
			sheetLevel={$sheetState}
			onStateChange={handleSheetStateChange}
		>
			{#snippet peekContent()}
				<div class="sheet-peek-row">
					<ConnectionStatus />
					<span class="station-count">{$stationList.length} stations</span>
				</div>
			{/snippet}

			{#if $panelMode === 'closed' || $panelMode === 'stations'}
				<StationList
					onSelect={handleStationClick}
					selectedKey={$selectedStation}
				/>
			{:else if $panelMode === 'detail' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
				/>
			{:else if $panelMode === 'messages'}
				<ConvoList
					onSelectConvo={handleConvoSelect}
				/>
			{:else if $panelMode === 'convo' && $selectedStation}
				<StationDetail
					stationKey={$selectedStation}
					activeTab={$detailTab}
					onTabChange={handleTabChange}
					onClose={closePanel}
					onFlyTo={handleFlyTo}
				/>
			{:else if $panelMode === 'transports'}
				<TransportPanel />
			{:else if $panelMode === 'activity'}
				<ActivityPanel />
			{:else if $panelMode === 'annotations'}
				<AnnotationPanel
					bind:this={annotationPanelRef}
					onFlyToAnnotation={handleFlyToAnnotation}
					onStartDraw={handleStartDraw}
					onPreviewChange={handlePreviewChange}
					onStartEdit={handleStartEdit}
					onStopEdit={handleStopEdit}
				/>
			{:else if $panelMode === 'bulletins'}
				<BulletinPanel />
			{:else if $panelMode === 'netcontrol'}
				<NetControlPanel
					onFlyTo={handleNetFlyTo}
					onSetOpsView={handleSetOpsView}
					onGoToOpsView={handleGoToOpsView}
				/>
			{:else if $panelMode === 'ics309'}
				<ICS309Panel />
			{/if}
		</BottomSheet>
	{/if}

	<!-- Mobile: Search Overlay -->
	{#if $searchOpen && !isDesktop}
		<SearchOverlay
			onClose={() => searchOpen.set(false)}
			onSelect={handleSearchSelect}
		/>
	{/if}
</div>

<style>
	.app-container {
		position: relative;
		width: 100vw;
		height: 100vh;
		height: 100dvh;
		overflow: hidden;
	}

	.map-layer {
		position: absolute;
		inset: 0;
		z-index: var(--z-map);
	}

	.sheet-peek-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding-bottom: var(--space-xs);
	}

	.station-count {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}
</style>
