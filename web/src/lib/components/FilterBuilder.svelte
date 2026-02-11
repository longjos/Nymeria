<script lang="ts">
	import { onMount, tick } from 'svelte';
	import type { FilterRule, FilterType } from '$lib/types';
	import {
		parseFilter, serializeFilter, validateRule, filterTypeLabel,
		ruleSummary, createDefaultRule, filterTypeGroups
	} from '$lib/filter-parser';

	let { value = $bindable(''), oninput }: { value: string; oninput?: (v: string) => void } = $props();

	let mode: 'visual' | 'advanced' = $state('visual');
	let rules: FilterRule[] = $state([]);
	let editingIndex: number | null = $state(null);
	let showTypePicker = $state(false);
	let rawValue = $state('');
	let parseError = $state<string | null>(null);
	let lastSyncedValue = $state(value);

	// Mini-map state — plain `let` (not $state) to avoid Svelte 5 deep-proxying
	// Leaflet objects, which breaks internal methods like containerPointToLatLng.
	let mapEl: HTMLDivElement | undefined;
	let mapInstance: any = null;
	let mapCircle: any = null;
	let mapRect: any = null;
	let mapMarker: any = null;
	let mapInitializing = false;
	let areaDrawStep: 'idle' | 'first' | 'done' = $state('idle');
	let L: any = null;

	// Parse incoming value on mount and when value changes externally
	onMount(() => {
		syncFromValue();
	});

	// Re-sync if value prop changes externally
	$effect(() => {
		if (value !== lastSyncedValue && value !== rawValue) {
			syncFromValue();
			lastSyncedValue = value;
		}
	});

	function syncFromValue() {
		try {
			rules = parseFilter(value);
			rawValue = value;
			parseError = null;
		} catch {
			parseError = 'Could not parse filter string';
			rules = [];
			rawValue = value;
		}
	}

	function syncToValue() {
		const newVal = serializeFilter(rules);
		value = newVal;
		rawValue = newVal;
		lastSyncedValue = newVal;
		oninput?.(newVal);
	}

	// Rule management
	function addRule(type: FilterType) {
		const rule = createDefaultRule(type);
		rules = [...rules, rule];
		editingIndex = rules.length - 1;
		showTypePicker = false;
		syncToValue();
	}

	function deleteRule(index: number) {
		if (editingIndex === index) {
			editingIndex = null;
			destroyMap();
		} else if (editingIndex != null && editingIndex > index) {
			editingIndex--;
		}
		rules = rules.filter((_, i) => i !== index);
		syncToValue();
	}

	function startEdit(index: number) {
		if (editingIndex !== null) {
			destroyMap();
		}
		editingIndex = index;
	}

	function finishEdit() {
		editingIndex = null;
		destroyMap();
		syncToValue();
	}

	function toggleExclude(index: number) {
		rules[index].exclude = !rules[index].exclude;
		rules = [...rules];
		syncToValue();
	}

	// Type filter helpers
	function typeHas(rule: FilterRule, code: string): boolean {
		return (rule.types || '').includes(code);
	}

	function toggleType(rule: FilterRule, code: string) {
		if (typeHas(rule, code)) {
			rule.types = (rule.types || '').replace(code, '');
		} else {
			rule.types = (rule.types || '') + code;
		}
		rules = [...rules];
		syncToValue();
	}

	// Q-construct helpers
	function qHas(rule: FilterRule, code: string): boolean {
		return (rule.qCodes || '').includes(code);
	}

	function toggleQ(rule: FilterRule, code: string) {
		if (qHas(rule, code)) {
			rule.qCodes = (rule.qCodes || '').replace(code, '');
		} else {
			rule.qCodes = (rule.qCodes || '') + code;
		}
		rules = [...rules];
		syncToValue();
	}

	// Items (callsigns, prefixes, etc.) as newline-separated text
	function itemsText(rule: FilterRule): string {
		return (rule.items || []).join('\n');
	}

	function setItems(rule: FilterRule, text: string) {
		rule.items = text.split('\n').map(s => s.trim()).filter(s => s.length > 0);
		rules = [...rules];
		syncToValue();
	}

	// Advanced mode
	function switchToAdvanced() {
		destroyMap();
		editingIndex = null;
		mode = 'advanced';
		rawValue = serializeFilter(rules);
	}

	function switchToVisual() {
		try {
			rules = parseFilter(rawValue);
			parseError = null;
			mode = 'visual';
			value = rawValue;
			oninput?.(rawValue);
		} catch {
			parseError = 'Could not parse filter string';
		}
	}

	function onRawInput() {
		value = rawValue;
		oninput?.(rawValue);
	}

	// Pointer event click detection — bypasses Leaflet's broken pointer-event
	// handling on iPadOS (same fix used in SetupWizard).
	let pointerDownPos: { x: number; y: number; time: number } | null = null;
	const CLICK_THRESHOLD = 10; // px
	const CLICK_TIME = 500;     // ms

	function onPointerDown(e: PointerEvent) {
		pointerDownPos = { x: e.clientX, y: e.clientY, time: Date.now() };
	}

	function onPointerUp(e: PointerEvent) {
		if (!pointerDownPos || !mapInstance || !mapEl) return;
		const dx = e.clientX - pointerDownPos.x;
		const dy = e.clientY - pointerDownPos.y;
		const dt = Date.now() - pointerDownPos.time;
		pointerDownPos = null;

		if (Math.sqrt(dx * dx + dy * dy) > CLICK_THRESHOLD || dt > CLICK_TIME) return;

		const target = e.target as HTMLElement;
		if (target.closest('.leaflet-control')) return;

		const rect = mapEl.getBoundingClientRect();
		const latlng = mapInstance.containerPointToLatLng(
			[e.clientX - rect.left, e.clientY - rect.top]
		);
		handleMapClick(latlng);
	}

	function handleMapClick(latlng: { lat: number; lng: number }) {
		if (editingIndex == null) return;
		const r = rules[editingIndex];
		if (r.type === 'range') {
			r.lat = parseFloat(latlng.lat.toFixed(4));
			r.lon = parseFloat(latlng.lng.toFixed(4));
			rules = [...rules];
			updateRangeOverlay(r);
			syncToValue();
		} else if (r.type === 'area') {
			const clickLat = parseFloat(latlng.lat.toFixed(4));
			const clickLon = parseFloat(latlng.lng.toFixed(4));
			if (areaDrawStep === 'idle' || areaDrawStep === 'done') {
				r.latN = clickLat;
				r.lonW = clickLon;
				r.latS = clickLat;
				r.lonE = clickLon;
				areaDrawStep = 'first';
			} else if (areaDrawStep === 'first') {
				const lat1 = r.latN!;
				const lon1 = r.lonW!;
				r.latN = Math.max(lat1, clickLat);
				r.latS = Math.min(lat1, clickLat);
				r.lonW = Math.min(lon1, clickLon);
				r.lonE = Math.max(lon1, clickLon);
				areaDrawStep = 'done';
			}
			rules = [...rules];
			updateAreaOverlay(r);
			syncToValue();
		}
	}

	// Mini-map for Range and Area filters
	async function initMap(rule: FilterRule) {
		if (mapInstance || mapInitializing) return;
		if (!mapEl) return;

		mapInitializing = true;
		try {
		// Dynamic import of Leaflet — use .default to get the L namespace
		// (matching static `import L from 'leaflet'` behavior)
		if (!L) {
			const mod = await import('leaflet');
			L = mod.default || mod;
		}

		const center: [number, number] = rule.type === 'range'
			? [rule.lat || 0, rule.lon || 0]
			: [((rule.latN || 0) + (rule.latS || 0)) / 2, ((rule.lonW || 0) + (rule.lonE || 0)) / 2];

		const zoom = rule.type === 'range' ? zoomForRadius(rule.dist || 100) : 6;

		mapInstance = L.map(mapEl, {
			center,
			zoom,
			zoomControl: true,
			attributionControl: false,
			tap: false
		});

		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			maxZoom: 18
		}).addTo(mapInstance);

		// Native pointer listeners — bypass Leaflet's broken click on iPadOS.
		// Attach BEFORE setup so they're always registered even if setup throws.
		mapEl.addEventListener('pointerdown', onPointerDown);
		mapEl.addEventListener('pointerup', onPointerUp);

		if (rule.type === 'range') {
			setupRangeMap(rule);
		} else if (rule.type === 'area') {
			setupAreaMap(rule);
		}

		// Fix map size after render
		await tick();
		requestAnimationFrame(() => {
			mapInstance?.invalidateSize();
		});
		} finally {
			mapInitializing = false;
		}
	}

	function setupRangeMap(rule: FilterRule) {
		if (!mapInstance || !L) return;
		const lat = rule.lat || 0;
		const lon = rule.lon || 0;
		const dist = rule.dist || 100;

		// Use a CSS divIcon — Leaflet's default PNG marker doesn't load in
		// bundled environments (Vite/SvelteKit). Same approach as SetupWizard.
		const crosshairIcon = L.divIcon({
			className: 'filter-crosshair',
			html: '<svg viewBox="0 0 24 24" width="24" height="24"><circle cx="12" cy="12" r="8" fill="none" stroke="#e94560" stroke-width="1.5" opacity="0.8"/><line x1="12" y1="2" x2="12" y2="8" stroke="#e94560" stroke-width="1.5"/><line x1="12" y1="16" x2="12" y2="22" stroke="#e94560" stroke-width="1.5"/><line x1="2" y1="12" x2="8" y2="12" stroke="#e94560" stroke-width="1.5"/><line x1="16" y1="12" x2="22" y2="12" stroke="#e94560" stroke-width="1.5"/><circle cx="12" cy="12" r="2" fill="#e94560"/></svg>',
			iconSize: [24, 24],
			iconAnchor: [12, 12],
		});
		mapMarker = L.marker([lat, lon], { draggable: true, icon: crosshairIcon }).addTo(mapInstance);
		mapCircle = L.circle([lat, lon], { radius: dist * 1000, color: '#e94560', fillOpacity: 0.1, weight: 2 }).addTo(mapInstance);

		mapMarker.on('dragend', () => {
			if (editingIndex == null) return;
			const pos = mapMarker.getLatLng();
			const r = rules[editingIndex];
			r.lat = parseFloat(pos.lat.toFixed(4));
			r.lon = parseFloat(pos.lng.toFixed(4));
			rules = [...rules];
			updateRangeOverlay(r);
			syncToValue();
		});
	}

	function updateRangeOverlay(rule: FilterRule) {
		if (!mapInstance || !L) return;
		const lat = rule.lat || 0;
		const lon = rule.lon || 0;
		const dist = rule.dist || 100;

		if (mapMarker) mapMarker.setLatLng([lat, lon]);
		if (mapCircle) {
			mapCircle.setLatLng([lat, lon]);
			mapCircle.setRadius(dist * 1000);
		}
	}

	function setupAreaMap(rule: FilterRule) {
		if (!mapInstance || !L) return;
		const bounds = L.latLngBounds(
			[rule.latS || -1, rule.lonW || -1],
			[rule.latN || 1, rule.lonE || 1]
		);
		mapRect = L.rectangle(bounds, { color: '#e94560', fillOpacity: 0.1, weight: 2 }).addTo(mapInstance);
		mapInstance.fitBounds(bounds.pad(0.2));
		// If bounds look like the default (very small), prompt for first click
		const area = Math.abs((rule.latN || 1) - (rule.latS || -1)) * Math.abs((rule.lonE || 1) - (rule.lonW || -1));
		areaDrawStep = area < 1 ? 'idle' : 'done';
	}

	function updateAreaOverlay(rule: FilterRule) {
		if (!mapInstance || !L) return;
		const bounds = L.latLngBounds(
			[rule.latS || -1, rule.lonW || -1],
			[rule.latN || 1, rule.lonE || 1]
		);
		if (mapRect) mapRect.setBounds(bounds);
	}

	function destroyMap() {
		if (mapEl) {
			mapEl.removeEventListener('pointerdown', onPointerDown);
			mapEl.removeEventListener('pointerup', onPointerUp);
		}
		if (mapInstance) {
			mapInstance.remove();
			mapInstance = null;
			mapCircle = null;
			mapRect = null;
			mapMarker = null;
		}
	}

	function zoomForRadius(km: number): number {
		if (km > 1000) return 4;
		if (km > 500) return 5;
		if (km > 200) return 6;
		if (km > 100) return 7;
		if (km > 50) return 8;
		if (km > 20) return 9;
		if (km > 10) return 10;
		return 11;
	}

	// Reactively manage mini-map — wait a tick for bind:this to populate mapEl
	$effect(() => {
		if (editingIndex != null) {
			const rule = rules[editingIndex];
			if (rule && (rule.type === 'range' || rule.type === 'area')) {
				tick().then(() => {
					if (mapEl) initMap(rule);
				});
			}
		}
	});

	// Packet type metadata
	const packetTypes = [
		{ code: 'p', label: 'Position' },
		{ code: 'o', label: 'Object' },
		{ code: 'i', label: 'Item' },
		{ code: 'm', label: 'Message' },
		{ code: 'w', label: 'Weather' },
		{ code: 'n', label: 'NWS Alert' },
		{ code: 't', label: 'Telemetry' },
		{ code: 's', label: 'Status' },
		{ code: 'q', label: 'Query' },
		{ code: 'u', label: 'User-defined' }
	];

	// Q-construct metadata
	const qTypes = [
		{ code: 'C', label: 'qAC — Direct client' },
		{ code: 'X', label: 'qAX — Unverified client' },
		{ code: 'U', label: 'qAU — UDP client' },
		{ code: 'R', label: 'qAR — Direct IGate' },
		{ code: 'r', label: 'qAr — Indirect IGate' },
		{ code: 'o', label: 'qAo — Client-only port' },
		{ code: 'O', label: 'qAO — Receive-only IGate' },
		{ code: 'S', label: 'qAS — Server' },
		{ code: 'I', label: 'qAI — Trace' },
		{ code: 'Z', label: 'qAZ — Server-generated' }
	];

	// Handle number input change for geo rules
	function onGeoChange(rule: FilterRule) {
		rules = [...rules];
		if (rule.type === 'range') updateRangeOverlay(rule);
		else if (rule.type === 'area') updateAreaOverlay(rule);
		syncToValue();
	}
</script>

<div class="filter-builder">
	<!-- Mode toggle -->
	<div class="mode-bar">
		<button
			class="mode-btn"
			class:active={mode === 'visual'}
			onclick={() => { if (mode !== 'visual') switchToVisual(); }}
		>Visual</button>
		<button
			class="mode-btn"
			class:active={mode === 'advanced'}
			onclick={() => { if (mode !== 'advanced') switchToAdvanced(); }}
		>Advanced</button>
	</div>

	{#if parseError}
		<div class="parse-error">{parseError}</div>
	{/if}

	{#if mode === 'advanced'}
		<!-- Raw filter string editor -->
		<div class="raw-editor">
			<textarea
				bind:value={rawValue}
				oninput={onRawInput}
				rows="3"
				placeholder="r/42/-71/100 t/p"
				spellcheck="false"
			></textarea>
			<div class="raw-hint">Space-separated APRS-IS filter commands. Prefix with - to exclude.</div>
		</div>
	{:else}
		<!-- Visual builder -->
		<div class="rule-list">
			{#each rules as rule, i}
				<div class="rule-card" class:editing={editingIndex === i}>
					<div class="rule-header">
						<span class="rule-type-badge">{filterTypeLabel(rule.type)}</span>
						{#if rule.exclude}
							<span class="exclude-badge">EXCLUDE</span>
						{/if}
						<div class="rule-actions">
							{#if editingIndex === i}
								<button class="rule-action-btn done-btn" onclick={finishEdit} title="Done editing">
									<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
										<path d="M3 8l4 4 6-7" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
									</svg>
								</button>
							{:else}
								<button class="rule-action-btn" onclick={() => startEdit(i)} title="Edit">
									<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
										<path d="M11 2l3 3-9 9H2v-3L11 2z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>
									</svg>
								</button>
							{/if}
							<button class="rule-action-btn delete-btn" onclick={() => deleteRule(i)} title="Remove">
								<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
									<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
								</svg>
							</button>
						</div>
					</div>

					{#if editingIndex !== i}
						<div class="rule-summary">{ruleSummary(rule)}</div>
					{:else}
						{@const err = validateRule(rule)}
						<!-- Edit form -->
						<div class="rule-edit">

							{#if rule.type === 'range'}
								<!-- Mini-map -->
								<div class="mini-map-container">
									<div class="mini-map" bind:this={mapEl}></div>
									<div class="map-hint">Click map to set center, drag marker to adjust</div>
								</div>
								<div class="edit-fields">
									<div class="field-group">
										<div class="field-row half">
											<label>Latitude</label>
											<input type="number" step="0.0001" min="-90" max="90" bind:value={rule.lat} oninput={() => onGeoChange(rule)} />
										</div>
										<div class="field-row half">
											<label>Longitude</label>
											<input type="number" step="0.0001" min="-180" max="180" bind:value={rule.lon} oninput={() => onGeoChange(rule)} />
										</div>
									</div>
									<div class="field-row">
										<label>Radius (km)</label>
										<input type="number" step="1" min="1" bind:value={rule.dist} oninput={() => onGeoChange(rule)} />
									</div>
								</div>

							{:else if rule.type === 'area'}
								<!-- Mini-map -->
								<div class="mini-map-container">
									<div class="mini-map" bind:this={mapEl}></div>
									<div class="map-hint">
										{#if areaDrawStep === 'idle'}Click to set first corner{:else if areaDrawStep === 'first'}Click to set opposite corner{:else}Click to redraw, or edit coordinates below{/if}
									</div>
								</div>
								<div class="edit-fields">
									<div class="field-group">
										<div class="field-row half">
											<label>North Lat</label>
											<input type="number" step="0.0001" min="-90" max="90" bind:value={rule.latN} oninput={() => onGeoChange(rule)} />
										</div>
										<div class="field-row half">
											<label>West Lon</label>
											<input type="number" step="0.0001" min="-180" max="180" bind:value={rule.lonW} oninput={() => onGeoChange(rule)} />
										</div>
									</div>
									<div class="field-group">
										<div class="field-row half">
											<label>South Lat</label>
											<input type="number" step="0.0001" min="-90" max="90" bind:value={rule.latS} oninput={() => onGeoChange(rule)} />
										</div>
										<div class="field-row half">
											<label>East Lon</label>
											<input type="number" step="0.0001" min="-180" max="180" bind:value={rule.lonE} oninput={() => onGeoChange(rule)} />
										</div>
									</div>
								</div>

							{:else if rule.type === 'type'}
								<div class="edit-fields">
									<div class="type-grid">
										{#each packetTypes as pt}
											<label class="type-chip" class:checked={typeHas(rule, pt.code)}>
												<input type="checkbox" checked={typeHas(rule, pt.code)} onchange={() => toggleType(rule, pt.code)} />
												<span>{pt.label}</span>
											</label>
										{/each}
									</div>
									<div class="field-row">
										<label>Near callsign (optional)</label>
										<input type="text" bind:value={rule.callForType} oninput={() => { rules = [...rules]; syncToValue(); }} placeholder="e.g. W1AW-5" />
									</div>
									{#if rule.callForType}
										<div class="field-row">
											<label>Distance (km)</label>
											<input type="number" step="1" min="1" bind:value={rule.distForType} oninput={() => { rules = [...rules]; syncToValue(); }} />
										</div>
									{/if}
								</div>

							{:else if rule.type === 'myRange'}
								<div class="edit-fields">
									<div class="field-row">
										<label>Distance from my station (km)</label>
										<input type="number" step="1" min="1" bind:value={rule.dist} oninput={() => { rules = [...rules]; syncToValue(); }} />
									</div>
									<div class="field-hint">Uses your station's last known APRS-IS position as center.</div>
								</div>

							{:else if rule.type === 'friendRange'}
								<div class="edit-fields">
									<div class="field-row">
										<label>Friend callsign</label>
										<input type="text" bind:value={rule.friendCall} oninput={() => { rules = [...rules]; syncToValue(); }} placeholder="e.g. W1AW-5" />
									</div>
									<div class="field-row">
										<label>Distance (km)</label>
										<input type="number" step="1" min="1" bind:value={rule.dist} oninput={() => { rules = [...rules]; syncToValue(); }} />
									</div>
									<div class="field-hint">Center follows the friend's position as they move.</div>
								</div>

							{:else if rule.type === 'prefix' || rule.type === 'budlist' || rule.type === 'object' || rule.type === 'strictObject' || rule.type === 'digipeater' || rule.type === 'entry' || rule.type === 'group' || rule.type === 'unproto'}
								<div class="edit-fields">
									<div class="field-row">
										<label>
											{#if rule.type === 'prefix'}Callsign prefixes{:else if rule.type === 'budlist'}Callsigns{:else if rule.type === 'object' || rule.type === 'strictObject'}Object names{:else if rule.type === 'digipeater'}Digipeater callsigns{:else if rule.type === 'entry'}IGate callsigns{:else if rule.type === 'group'}Message recipients{:else}Destination callsigns{/if}
											<span class="label-hint">(one per line, * wildcard supported)</span>
										</label>
										<textarea
											rows="4"
											value={itemsText(rule)}
											oninput={(e) => setItems(rule, (e.target as HTMLTextAreaElement).value)}
											spellcheck="false"
											placeholder={rule.type === 'prefix' ? 'W\nK\nN' : rule.type === 'budlist' ? 'W1AW-5\nN5JXS*' : 'BLN*\nNWS*'}
										></textarea>
									</div>
								</div>

							{:else if rule.type === 'symbol'}
								<div class="edit-fields">
									<div class="field-row">
										<label>Primary table symbols</label>
										<input type="text" bind:value={rule.primaryTable} oninput={() => { rules = [...rules]; syncToValue(); }} placeholder="-> (house, car)" />
									</div>
									<div class="field-row">
										<label>Alternate table symbols</label>
										<input type="text" bind:value={rule.altTable} oninput={() => { rules = [...rules]; syncToValue(); }} placeholder="# (digi)" />
									</div>
									<div class="field-row">
										<label>Overlay characters</label>
										<input type="text" bind:value={rule.overlay} oninput={() => { rules = [...rules]; syncToValue(); }} placeholder="A-Z, 0-9" />
									</div>
								</div>

							{:else if rule.type === 'qConstruct'}
								<div class="edit-fields">
									<div class="type-grid">
										{#each qTypes as qt}
											<label class="type-chip" class:checked={qHas(rule, qt.code)}>
												<input type="checkbox" checked={qHas(rule, qt.code)} onchange={() => toggleQ(rule, qt.code)} />
												<span>{qt.label}</span>
											</label>
										{/each}
									</div>
									<label class="type-chip" class:checked={rule.iFlag} style="margin-top: var(--space-xs)">
										<input type="checkbox" bind:checked={rule.iFlag} onchange={() => { rules = [...rules]; syncToValue(); }} />
										<span>Include IGate positions</span>
									</label>
								</div>
							{/if}

							<!-- Exclude toggle -->
							<div class="exclude-row">
								<label class="exclude-toggle">
									<input type="checkbox" checked={rule.exclude} onchange={() => toggleExclude(i)} />
									<span>Exclude (invert this filter)</span>
								</label>
							</div>

							{#if err}
								<div class="validation-error">{err}</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}

			{#if rules.length === 0 && !showTypePicker}
				<div class="empty-state">
					No filters configured. All APRS-IS traffic on port 14580 will be limited to messages addressed to your station.
				</div>
			{/if}

			<!-- Add filter button / type picker -->
			{#if showTypePicker}
				<div class="type-picker">
					{#each filterTypeGroups as group}
						<div class="type-group">
							<div class="type-group-label">{group.label}</div>
							<div class="type-group-options">
								{#each group.types as ft}
									<button class="type-pick-btn" onclick={() => addRule(ft)}>
										{filterTypeLabel(ft)}
									</button>
								{/each}
							</div>
						</div>
					{/each}
					<button class="cancel-pick-btn" onclick={() => { showTypePicker = false; }}>Cancel</button>
				</div>
			{:else}
				<button class="add-rule-btn" onclick={() => { showTypePicker = true; }}>
					+ Add Filter Rule
				</button>
			{/if}
		</div>

		<!-- Generated filter preview -->
		{#if rules.length > 0}
			<div class="filter-preview">
				<span class="preview-label">Filter:</span>
				<code class="preview-value">{serializeFilter(rules)}</code>
			</div>
		{/if}
	{/if}
</div>

<style>
	.filter-builder {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	/* Mode toggle bar */
	.mode-bar {
		display: flex;
		gap: 2px;
		background: var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 2px;
	}

	.mode-btn {
		flex: 1;
		padding: 4px 8px;
		font-size: 0.72rem;
		font-weight: 600;
		background: none;
		border: none;
		border-radius: 4px;
		color: var(--color-text-muted);
		cursor: pointer;
		transition: background var(--duration-fast), color var(--duration-fast);
	}

	.mode-btn.active {
		background: var(--color-accent);
		color: white;
	}

	.mode-btn:hover:not(.active) {
		color: var(--color-text);
	}

	/* Parse error */
	.parse-error {
		padding: 4px 8px;
		background: rgba(239, 68, 68, 0.1);
		border: 1px solid rgba(239, 68, 68, 0.3);
		border-radius: var(--radius-sm);
		color: #f87171;
		font-size: 0.72rem;
	}

	/* Raw editor */
	.raw-editor textarea {
		width: 100%;
		padding: 8px 10px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.82rem;
		font-family: monospace;
		resize: vertical;
		outline: none;
	}

	.raw-editor textarea:focus {
		border-color: var(--color-accent);
	}

	.raw-hint {
		font-size: 0.68rem;
		color: var(--color-text-muted);
		margin-top: 2px;
	}

	/* Rule list */
	.rule-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.rule-card {
		background: rgba(255, 255, 255, 0.02);
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: var(--radius-sm);
		padding: var(--space-xs) var(--space-sm);
		transition: border-color var(--duration-fast);
	}

	.rule-card.editing {
		border-color: rgba(233, 69, 96, 0.3);
	}

	.rule-header {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		min-height: 24px;
	}

	.rule-type-badge {
		font-size: 0.68rem;
		font-weight: 700;
		color: var(--color-accent);
		letter-spacing: 0.02em;
		white-space: nowrap;
	}

	.exclude-badge {
		font-size: 0.58rem;
		font-weight: 700;
		padding: 1px 4px;
		border-radius: 3px;
		background: rgba(239, 68, 68, 0.15);
		color: #f87171;
		letter-spacing: 0.04em;
	}

	.rule-actions {
		margin-left: auto;
		display: flex;
		gap: 2px;
	}

	.rule-action-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 22px;
		height: 22px;
		background: none;
		border: none;
		border-radius: 4px;
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.rule-action-btn:hover {
		color: var(--color-text);
		background: rgba(255, 255, 255, 0.06);
	}

	.done-btn:hover {
		color: var(--color-success);
		background: rgba(34, 197, 94, 0.1);
	}

	.delete-btn:hover {
		color: #f87171;
		background: rgba(239, 68, 68, 0.1);
	}

	.rule-summary {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		margin-top: 2px;
		line-height: 1.3;
	}

	/* Edit form */
	.rule-edit {
		margin-top: var(--space-xs);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.edit-fields {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.field-group {
		display: flex;
		gap: var(--space-xs);
	}

	.field-row {
		display: flex;
		flex-direction: column;
	}

	.field-row.half {
		flex: 1;
	}

	.field-row label {
		font-size: 0.68rem;
		color: var(--color-text-muted);
		margin-bottom: 2px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.label-hint {
		text-transform: none;
		letter-spacing: normal;
		font-weight: normal;
	}

	.field-row input[type="text"],
	.field-row input[type="number"],
	.field-row textarea {
		width: 100%;
		padding: 5px 8px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.78rem;
		font-family: inherit;
		outline: none;
	}

	.field-row textarea {
		font-family: monospace;
		resize: vertical;
	}

	.field-row input:focus,
	.field-row textarea:focus {
		border-color: var(--color-accent);
	}

	.field-hint {
		font-size: 0.66rem;
		color: var(--color-text-muted);
		font-style: italic;
	}

	/* Type checkboxes grid */
	.type-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}

	.type-chip {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 3px 8px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		cursor: pointer;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		transition: border-color var(--duration-fast), color var(--duration-fast);
		user-select: none;
	}

	.type-chip input[type="checkbox"] {
		display: none;
	}

	.type-chip.checked {
		border-color: rgba(233, 69, 96, 0.4);
		color: var(--color-text);
		background: rgba(233, 69, 96, 0.08);
	}

	.type-chip:hover {
		border-color: rgba(255, 255, 255, 0.15);
		color: var(--color-text);
	}

	/* Exclude toggle */
	.exclude-row {
		margin-top: var(--space-xs);
		padding-top: var(--space-xs);
		border-top: 1px solid rgba(255, 255, 255, 0.04);
	}

	.exclude-toggle {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		cursor: pointer;
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.exclude-toggle input[type="checkbox"] {
		width: 14px;
		height: 14px;
		accent-color: var(--color-accent);
	}

	/* Validation error */
	.validation-error {
		font-size: 0.68rem;
		color: #f87171;
		padding: 2px 0;
	}

	/* Empty state */
	.empty-state {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		padding: var(--space-sm);
		text-align: center;
		font-style: italic;
	}

	/* Add rule button */
	.add-rule-btn {
		width: 100%;
		padding: 6px;
		background: none;
		border: 1px dashed rgba(255, 255, 255, 0.12);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.72rem;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.add-rule-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	/* Type picker */
	.type-picker {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-xs);
		background: rgba(255, 255, 255, 0.02);
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: var(--radius-sm);
	}

	.type-group-label {
		font-size: 0.62rem;
		font-weight: 700;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-bottom: 2px;
	}

	.type-group-options {
		display: flex;
		flex-wrap: wrap;
		gap: 3px;
		margin-bottom: var(--space-xs);
	}

	.type-pick-btn {
		padding: 3px 8px;
		font-size: 0.7rem;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: 4px;
		color: var(--color-text);
		cursor: pointer;
		transition: border-color var(--duration-fast);
	}

	.type-pick-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.cancel-pick-btn {
		padding: 4px 8px;
		font-size: 0.7rem;
		background: none;
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: 4px;
		color: var(--color-text-muted);
		cursor: pointer;
		align-self: flex-end;
	}

	/* Mini-map */
	.mini-map-container {
		margin-bottom: var(--space-xs);
	}

	.mini-map {
		width: 100%;
		height: 180px;
		border-radius: var(--radius-sm);
		border: 1px solid rgba(255, 255, 255, 0.08);
		z-index: 0;
		touch-action: none;
	}

	.map-hint {
		font-size: 0.64rem;
		color: var(--color-text-muted);
		margin-top: 2px;
		text-align: center;
	}

	/* Crosshair marker for range center */
	:global(.filter-crosshair) {
		background: none !important;
		border: none !important;
	}

	/* Filter preview */
	.filter-preview {
		display: flex;
		align-items: flex-start;
		gap: var(--space-xs);
		padding: 4px 8px;
		background: rgba(255, 255, 255, 0.02);
		border-radius: var(--radius-sm);
		border: 1px solid rgba(255, 255, 255, 0.04);
	}

	.preview-label {
		font-size: 0.66rem;
		color: var(--color-text-muted);
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		white-space: nowrap;
		padding-top: 1px;
	}

	.preview-value {
		font-size: 0.72rem;
		color: var(--color-text);
		font-family: monospace;
		word-break: break-all;
		line-height: 1.4;
	}
</style>
