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
	import WeatherPanel from '$lib/components/WeatherPanel.svelte';
	import SettingsPanel from '$lib/components/SettingsPanel.svelte';
	import ConnectionStatus from '$lib/components/ConnectionStatus.svelte';
	import SearchOverlay from '$lib/components/SearchOverlay.svelte';
	import LoginOverlay from '$lib/components/LoginOverlay.svelte';
	import CommandPalette from '$lib/components/CommandPalette.svelte';
	import { stations, stationList, initStationStore } from '$lib/stores/stations';
	import { initMessageStore, conversationList } from '$lib/stores/messages';
	import { initTransportStore } from '$lib/stores/transports';
	import { annotationList, initAnnotationStore } from '$lib/stores/annotations';
	import { api } from '$lib/api';
	import {
		initNetControlStore,
		activeNet, operatorsWithPosition, missionsWithPosition, assignmentLines,
		opsView, hoveredMissionId, hoveredCheckInId
	} from '$lib/stores/netcontrol';
	import { initTacticalStore } from '$lib/stores/tactical';
	import { initBulletinStore } from '$lib/stores/bulletins';
	import { initWeatherStore, weatherStations, selectedWeatherStation } from '$lib/stores/weather';
	import { isLoggedIn, initSession } from '$lib/stores/session';
	import {
		selectedStation, panelMode, detailTab, searchOpen, sheetState,
		selectStation, closePanel, openStationList, openMessages, openConversation, openTransports, openActivity, openAnnotations, openNetControl, openBulletins, openICS309, openWeather, openSettings,
		togglePanel, commandPaletteOpen, toggleCommandPalette
	} from '$lib/stores/ui';
	import type { SheetState, DetailTab, PanelMode } from '$lib/stores/ui';
	import type { Annotation } from '$lib/types';

	let isDesktop = $state(true);
	let flyToTarget = $state<{ lat: number; lon: number; zoom?: number } | null>(null);
	let flyToBounds = $state<Array<{ lat: number; lon: number }> | null>(null);
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
			initWeatherStore();
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
		const st = $stations.get(key);
		// Weather stations → open the weather panel detail instead
		if (st?.weather) {
			selectedWeatherStation.set(key);
			openWeather();
			return;
		}
		selectStation(key);
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

	function handleFlyToBounds(coords: Array<{ lat: number; lon: number }>) {
		flyToBounds = coords;
		setTimeout(() => { flyToBounds = null; }, 100);
	}

	async function handleSetOpsView() {
		const vp = mapRef?.getViewport();
		if (vp && $activeNet) {
			opsView.set(vp);
			try {
				await api.setOpsView($activeNet.id, vp.lat, vp.lon, vp.zoom);
			} catch { /* best-effort persist */ }
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

	function handleGlobalKeydown(e: KeyboardEvent) {
		// Ctrl+K / Cmd+K → toggle command palette
		if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
			e.preventDefault();
			toggleCommandPalette();
			return;
		}
		// '/' when not in input/textarea → open command palette
		if (e.key === '/' && !$commandPaletteOpen) {
			const tag = (e.target as HTMLElement)?.tagName;
			if (tag !== 'INPUT' && tag !== 'TEXTAREA' && !(e.target as HTMLElement)?.isContentEditable) {
				e.preventDefault();
				commandPaletteOpen.set(true);
			}
		}
	}

	function handlePaletteFlyTo(lat: number, lon: number) {
		flyToTarget = { lat, lon, zoom: 15 };
	}
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

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
			{flyToBounds}
			highlightedMissionId={$hoveredMissionId}
			highlightedCheckInId={$hoveredCheckInId}
			weatherOverlay={$weatherStations}
			showWeatherOverlay={$panelMode === 'weather'}
		/>
	</div>

	<!-- Floating Ops View restore button -->
	{#if $opsView && $activeNet}
		<button class="ops-view-fab" onclick={handleGoToOpsView} title="Return to Ops View">
			<svg width="18" height="18" viewBox="0 0 16 16" fill="none">
				<circle cx="8" cy="7" r="3" stroke="currentColor" stroke-width="1.5"/>
				<path d="M8 1C4.5 1 1.5 3.5 1 7c.5 3.5 3.5 6 7 6s6.5-2.5 7-6c-.5-3.5-3.5-6-7-6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
				<path d="M8 13v2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			</svg>
		</button>
	{/if}

	<!-- Desktop: Activity Rail (right edge) -->
	{#if isDesktop}
		<ActivityRail
			panelMode={$panelMode}
			unreadCount={totalUnread}
			onToggle={handleRailToggle}
			onSelectStation={handleSearchSelect}
			onCommandPalette={toggleCommandPalette}
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
		onWeatherOpen={openWeather}
		onSettingsOpen={openSettings}
		onCommandPalette={toggleCommandPalette}
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
					onFlyToBounds={handleFlyToBounds}
					onSetOpsView={handleSetOpsView}
					onGoToOpsView={handleGoToOpsView}
				/>
			{:else if $panelMode === 'weather'}
				<WeatherPanel onFlyTo={handleFlyTo} />
			{:else if $panelMode === 'ics309'}
				<ICS309Panel />
			{:else if $panelMode === 'settings'}
				<SettingsPanel />
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
					onFlyToBounds={handleFlyToBounds}
					onSetOpsView={handleSetOpsView}
					onGoToOpsView={handleGoToOpsView}
				/>
			{:else if $panelMode === 'weather'}
				<WeatherPanel onFlyTo={handleFlyTo} />
			{:else if $panelMode === 'ics309'}
				<ICS309Panel />
			{:else if $panelMode === 'settings'}
				<SettingsPanel />
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

	<!-- Command Palette (Ctrl+K) -->
	{#if $commandPaletteOpen}
		<CommandPalette
			onFlyTo={handlePaletteFlyTo}
			onClose={() => commandPaletteOpen.set(false)}
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

	.ops-view-fab {
		position: absolute;
		top: 80px;
		left: 10px;
		z-index: var(--z-toolbar);
		width: 40px;
		height: 40px;
		border-radius: 8px;
		border: 2px solid #22c55e;
		background: var(--color-surface);
		background: rgba(var(--color-surface-rgb, 30, 30, 30), 0.9);
		color: #22c55e;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
		transition: background 0.15s, border-color 0.15s;
	}

	.ops-view-fab:hover {
		background: rgba(34, 197, 94, 0.15);
		border-color: #4ade80;
	}
</style>
