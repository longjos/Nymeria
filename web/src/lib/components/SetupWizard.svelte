<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { needsSetup } from '$lib/stores/session';
	import type { SetupData } from '$lib/types';
	import L from 'leaflet';

	let step = $state(0);
	const TOTAL_STEPS = 5;

	// Step 1: Station
	let callsign = $state('');
	let ssid = $state(0);
	let comment = $state('Nymeria APRS Client');

	// Step 2: Location
	let lat = $state(39.83);
	let lon = $state(-98.58);
	let mapContainer: HTMLDivElement;
	let leafletMap: L.Map | null = null;
	let marker: L.Marker | null = null;

	// Step 3: APRS-IS
	let aprisEnabled = $state(true);
	let aprisHost = $state('rotate.aprs2.net');
	let aprisPort = $state(14580);
	let aprisFilter = $derived(`r/${lat.toFixed(2)}/${lon.toFixed(2)}/200`);

	// Step 4: Review + submit
	let saving = $state(false);
	let saveError = $state('');
	let saveSuccess = $state(false);

	// Validation
	let callsignValid = $derived(/^[A-Za-z0-9]{3,9}$/.test(callsign.trim()));
	let latValid = $derived(lat >= -90 && lat <= 90);
	let lonValid = $derived(lon >= -180 && lon <= 180);

	let canNext = $derived.by(() => {
		switch (step) {
			case 1: return callsignValid;
			case 2: return latValid && lonValid;
			case 3: return true;
			default: return true;
		}
	});

	// APRS-IS passcode (client-side computation matching server's algorithm)
	let passcode = $derived.by(() => {
		const call = callsign.trim().toUpperCase().split('-')[0];
		if (call.length < 3) return '—';
		let hash = 0x73E2;
		for (let i = 0; i < call.length; i += 2) {
			hash ^= call.charCodeAt(i) << 8;
			if (i + 1 < call.length) {
				hash ^= call.charCodeAt(i + 1);
			}
		}
		return String(hash & 0x7FFF);
	});

	const SSID_LABELS: Record<number, string> = {
		0: 'Primary',
		1: 'Generic 1',
		2: 'Generic 2',
		3: 'Generic 3',
		4: 'Generic 4',
		5: 'Smartphone',
		6: 'Satellite',
		7: 'Handheld',
		8: 'Boat/Ship',
		9: 'Vehicle',
		10: 'Internet',
		11: 'Balloon',
		12: 'APRStt',
		13: 'Weather',
		14: 'Trucking',
		15: 'HF Gateway'
	};

	function next() {
		if (step < TOTAL_STEPS - 1) step++;
	}

	function back() {
		if (step > 0) step--;
	}

	async function save() {
		saving = true;
		saveError = '';
		try {
			const data: SetupData = {
				callsign: callsign.trim().toUpperCase(),
				ssid,
				comment: comment.trim(),
				lat,
				lon,
				aprisEnabled,
				aprisHost: aprisEnabled ? aprisHost : '',
				aprisPort: aprisEnabled ? aprisPort : 0,
				aprisFilter: aprisEnabled ? aprisFilter : ''
			};
			await api.setup(data);
			saveSuccess = true;
		} catch (err) {
			saveError = err instanceof Error ? err.message : 'Setup failed';
		} finally {
			saving = false;
		}
	}

	function handleCallsignInput(e: Event) {
		const input = e.target as HTMLInputElement;
		input.value = input.value.toUpperCase();
		callsign = input.value;
	}

	// Track pointer state for our own click detection (bypasses Leaflet's
	// broken pointer-event handling on iPadOS).
	let pointerDownPos: { x: number; y: number; time: number } | null = null;
	const CLICK_THRESHOLD = 10; // px — ignore if finger/cursor moved more than this
	const CLICK_TIME = 500;     // ms — ignore if held longer than this

	function onPointerDown(e: PointerEvent) {
		pointerDownPos = { x: e.clientX, y: e.clientY, time: Date.now() };
	}

	function onPointerUp(e: PointerEvent) {
		if (!pointerDownPos || !leafletMap || !marker) return;
		const dx = e.clientX - pointerDownPos.x;
		const dy = e.clientY - pointerDownPos.y;
		const dt = Date.now() - pointerDownPos.time;
		pointerDownPos = null;

		// Only treat as a click if pointer barely moved and was quick
		if (Math.sqrt(dx * dx + dy * dy) > CLICK_THRESHOLD || dt > CLICK_TIME) return;

		// Don't fire if the tap was on a Leaflet control (zoom buttons, etc.)
		const target = e.target as HTMLElement;
		if (target.closest('.leaflet-control')) return;

		// Convert screen coordinates → map lat/lng
		const rect = mapContainer.getBoundingClientRect();
		const point = L.point(e.clientX - rect.left, e.clientY - rect.top);
		const latlng = leafletMap.containerPointToLatLng(point);

		lat = Math.round(latlng.lat * 1e6) / 1e6;
		lon = Math.round(latlng.lng * 1e6) / 1e6;
		marker.setLatLng(latlng);
	}

	function initMap() {
		if (!mapContainer || leafletMap) return;
		leafletMap = L.map(mapContainer, {
			center: [lat, lon],
			zoom: 4,
			zoomControl: true,
			attributionControl: false,
			// Disable Leaflet's legacy tap handler — it's unreliable on iOS/iPad.
			tap: false
		});

		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			maxZoom: 18,
		}).addTo(leafletMap);

		// Use a CSS DivIcon instead of Leaflet's default image marker,
		// which fails to load in bundled environments (Vite/SvelteKit).
		const pinIcon = L.divIcon({
			className: 'setup-marker',
			html: '<div class="setup-marker-pin"></div><div class="setup-marker-shadow"></div>',
			iconSize: [24, 36],
			iconAnchor: [12, 36],
		});
		marker = L.marker([lat, lon], { draggable: true, icon: pinIcon }).addTo(leafletMap);

		marker.on('dragend', () => {
			const pos = marker!.getLatLng();
			lat = Math.round(pos.lat * 1e6) / 1e6;
			lon = Math.round(pos.lng * 1e6) / 1e6;
		});

		// Native pointer event listeners — these bypass Leaflet's event system
		// entirely, which is necessary because Leaflet 1.9.x has a known bug
		// on iPadOS where its internal `_moved` flag gets set incorrectly from
		// pointer events, suppressing the `click` event.
		mapContainer.addEventListener('pointerdown', onPointerDown);
		mapContainer.addEventListener('pointerup', onPointerUp);
	}

	function destroyMap() {
		if (mapContainer) {
			mapContainer.removeEventListener('pointerdown', onPointerDown);
			mapContainer.removeEventListener('pointerup', onPointerUp);
		}
		if (leafletMap) {
			leafletMap.remove();
			leafletMap = null;
			marker = null;
		}
	}

	// Initialize/destroy map when entering/leaving step 2
	$effect(() => {
		if (step === 2) {
			// Delay to ensure container is rendered
			requestAnimationFrame(() => {
				initMap();
				if (leafletMap) leafletMap.invalidateSize();
			});
		} else {
			destroyMap();
		}
	});

	// Keep marker synced with manual lat/lon input
	$effect(() => {
		if (marker && leafletMap) {
			marker.setLatLng([lat, lon]);
		}
	});

	onDestroy(() => {
		destroyMap();
	});
</script>

<div class="wizard-overlay">
	<div class="wizard-card">
		<!-- Step indicator -->
		{#if step > 0 && !saveSuccess}
			<div class="step-indicator">
				{#each Array(TOTAL_STEPS - 1) as _, i}
					<div
						class="step-dot"
						class:active={step === i + 1}
						class:done={step > i + 1}
					></div>
				{/each}
			</div>
		{/if}

		<!-- Step 0: Welcome -->
		{#if step === 0}
			<div class="step welcome-step">
				<div class="logo-area">
					<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="1.5">
						<circle cx="12" cy="12" r="10"/>
						<path d="M12 2a15 15 0 0 1 4 10 15 15 0 0 1-4 10 15 15 0 0 1-4-10A15 15 0 0 1 12 2z"/>
						<path d="M2 12h20"/>
					</svg>
					<h1>Nymeria</h1>
				</div>
				<p class="welcome-text">Welcome to Nymeria, your APRS client. Let's get you on the air in about 60 seconds.</p>
				<button class="btn-primary" onclick={next}>Get Started</button>
			</div>

		<!-- Step 1: Station Identity -->
		{:else if step === 1}
			<div class="step">
				<h2>Station Identity</h2>
				<p class="step-desc">Your FCC callsign and station configuration.</p>

				<label class="field">
					<span>Callsign</span>
					<input
						type="text"
						value={callsign}
						oninput={handleCallsignInput}
						placeholder="e.g. KD7BBC"
						maxlength="9"
						autofocus
						class:invalid={callsign.length > 0 && !callsignValid}
					/>
					{#if callsign.length > 0 && !callsignValid}
						<small class="field-error">3-9 alphanumeric characters</small>
					{/if}
				</label>

				<label class="field">
					<span>SSID</span>
					<select bind:value={ssid}>
						{#each Array(16) as _, i}
							<option value={i}>{i} — {SSID_LABELS[i]}</option>
						{/each}
					</select>
				</label>

				<label class="field">
					<span>Status Comment</span>
					<input
						type="text"
						bind:value={comment}
						placeholder="e.g. Nymeria APRS Client"
						maxlength="43"
					/>
				</label>
			</div>

		<!-- Step 2: Location -->
		{:else if step === 2}
			<div class="step location-step">
				<h2>Station Location</h2>
				<p class="step-desc">Click the map or enter coordinates. This sets your APRS position and filter center.</p>

				<div class="map-wrapper" bind:this={mapContainer}></div>

				<div class="coord-row">
					<label class="field coord-field">
						<span>Latitude</span>
						<input
							type="number"
							bind:value={lat}
							step="0.01"
							min="-90"
							max="90"
							class:invalid={!latValid}
						/>
					</label>
					<label class="field coord-field">
						<span>Longitude</span>
						<input
							type="number"
							bind:value={lon}
							step="0.01"
							min="-180"
							max="180"
							class:invalid={!lonValid}
						/>
					</label>
				</div>
			</div>

		<!-- Step 3: APRS-IS -->
		{:else if step === 3}
			<div class="step">
				<h2>APRS-IS Connection</h2>
				<p class="step-desc">Connect to the APRS Internet System to see stations worldwide.</p>

				<label class="toggle-row">
					<input type="checkbox" bind:checked={aprisEnabled} />
					<span>Enable APRS-IS</span>
				</label>

				{#if aprisEnabled}
					<div class="apris-fields">
						<label class="field">
							<span>Server</span>
							<input type="text" bind:value={aprisHost} placeholder="rotate.aprs2.net" />
						</label>

						<label class="field">
							<span>Port</span>
							<input type="number" bind:value={aprisPort} min="1" max="65535" />
						</label>

						<label class="field">
							<span>Filter</span>
							<input type="text" value={aprisFilter} readonly />
							<small class="field-hint">Auto-computed from your location</small>
						</label>

						<div class="passcode-display">
							<span class="passcode-label">Passcode</span>
							<span class="passcode-value">{passcode}</span>
							<small class="field-hint">Auto-computed from callsign</small>
						</div>
					</div>
				{:else}
					<p class="skip-hint">You can enable this later in Settings. Without APRS-IS, you'll only see stations via local RF (if a TNC is connected).</p>
				{/if}
			</div>

		<!-- Step 4: Review -->
		{:else if step === 4}
			<div class="step">
				{#if saveSuccess}
					<div class="success-state">
						<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="#22c55e" stroke-width="2">
							<circle cx="12" cy="12" r="10"/>
							<path d="M8 12l3 3 5-6"/>
						</svg>
						<h2>Config Saved</h2>
						<p>Restart Nymeria to apply the new configuration and connect to APRS-IS.</p>
						<p class="restart-hint">Run: <code>nymeria --listen :9090</code></p>
					</div>
				{:else}
					<h2>Review &amp; Save</h2>
					<p class="step-desc">Confirm your settings before saving.</p>

					<div class="review-card">
						<div class="review-row">
							<span class="review-label">Callsign</span>
							<span class="review-value">{callsign.toUpperCase()}{ssid > 0 ? `-${ssid}` : ''}</span>
						</div>
						<div class="review-row">
							<span class="review-label">Comment</span>
							<span class="review-value">{comment || '—'}</span>
						</div>
						<div class="review-row">
							<span class="review-label">Location</span>
							<span class="review-value">{lat.toFixed(4)}, {lon.toFixed(4)}</span>
						</div>
						<div class="review-row">
							<span class="review-label">APRS-IS</span>
							<span class="review-value">
								{#if aprisEnabled}
									{aprisHost}:{aprisPort}
								{:else}
									Disabled
								{/if}
							</span>
						</div>
						{#if aprisEnabled}
							<div class="review-row">
								<span class="review-label">Filter</span>
								<span class="review-value">{aprisFilter}</span>
							</div>
						{/if}
					</div>

					{#if saveError}
						<p class="error">{saveError}</p>
					{/if}

					<button class="btn-primary" onclick={save} disabled={saving}>
						{#if saving}
							Saving...
						{:else}
							Save &amp; Launch
						{/if}
					</button>
				{/if}
			</div>
		{/if}

		<!-- Navigation -->
		{#if step > 0 && step < TOTAL_STEPS - 1}
			<div class="nav-row">
				<button class="btn-ghost" onclick={back}>Back</button>
				<button class="btn-primary" onclick={next} disabled={!canNext}>Next</button>
			</div>
		{:else if step === TOTAL_STEPS - 1 && !saveSuccess}
			<div class="nav-row">
				<button class="btn-ghost" onclick={back}>Back</button>
			</div>
		{/if}
	</div>
</div>

<style>
	.wizard-overlay {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--color-bg);
		overflow-y: auto;
		padding: var(--space-md);
	}

	.wizard-card {
		width: 100%;
		max-width: 480px;
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.step-indicator {
		display: flex;
		justify-content: center;
		gap: var(--space-sm);
	}

	.step-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--color-primary);
		opacity: 0.3;
		transition: opacity 0.2s, background 0.2s;
	}

	.step-dot.active {
		opacity: 1;
		background: var(--color-accent);
	}

	.step-dot.done {
		opacity: 0.7;
		background: var(--color-accent);
	}

	.step {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	/* Welcome */
	.welcome-step {
		align-items: center;
		text-align: center;
		padding: var(--space-xl) 0;
	}

	.logo-area {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		color: var(--color-accent);
		margin-bottom: var(--space-sm);
	}

	.logo-area h1 {
		font-size: 2rem;
		font-weight: 700;
		letter-spacing: 0.05em;
	}

	.welcome-text {
		font-size: 0.95rem;
		color: var(--color-text-muted);
		max-width: 320px;
		line-height: 1.5;
	}

	/* Step titles */
	h2 {
		font-size: 1.15rem;
		font-weight: 600;
		margin: 0;
	}

	.step-desc {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		margin: -4px 0 var(--space-xs) 0;
		line-height: 1.4;
	}

	/* Fields */
	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field span {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.field input, .field select {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text);
		padding: 10px 12px;
		font-size: 0.9rem;
		outline: none;
		transition: border-color 0.15s;
	}

	.field input:focus, .field select:focus {
		border-color: var(--color-accent);
	}

	.field input.invalid {
		border-color: var(--color-error);
	}

	.field input::placeholder {
		color: var(--color-text-muted);
		opacity: 0.5;
	}

	.field input[readonly] {
		opacity: 0.7;
	}

	.field-error {
		font-size: 0.75rem;
		color: var(--color-error);
	}

	.field-hint {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		opacity: 0.6;
	}

	/* Location step */
	.location-step {
		gap: var(--space-sm);
	}

	.map-wrapper {
		width: 100%;
		height: 260px;
		border-radius: var(--radius-md);
		border: 1px solid var(--color-primary);
		overflow: hidden;
		/* Prevent the scrollable wizard overlay from stealing touch events */
		touch-action: none;
	}

	.coord-row {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-sm);
	}

	.coord-field input {
		width: 100%;
	}

	/* APRS-IS toggle */
	.toggle-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		cursor: pointer;
		font-size: 0.9rem;
	}

	.toggle-row input[type="checkbox"] {
		width: 18px;
		height: 18px;
		accent-color: var(--color-accent);
	}

	.apris-fields {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.passcode-display {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.passcode-label {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.passcode-value {
		font-family: monospace;
		font-size: 1.1rem;
		color: var(--color-accent);
		letter-spacing: 0.1em;
	}

	.skip-hint {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		line-height: 1.5;
		padding: var(--space-sm) 0;
	}

	/* Review card */
	.review-card {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.review-row {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: var(--space-sm);
	}

	.review-label {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.review-value {
		font-size: 0.9rem;
		text-align: right;
		word-break: break-all;
	}

	/* Success state */
	.success-state {
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-md);
		padding: var(--space-lg) 0;
	}

	.success-state h2 {
		color: #22c55e;
	}

	.success-state p {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		line-height: 1.5;
		max-width: 320px;
	}

	.restart-hint {
		margin-top: var(--space-xs);
	}

	.restart-hint code {
		background: var(--color-surface);
		padding: 2px 8px;
		border-radius: var(--radius-sm, 4px);
		font-size: 0.85rem;
	}

	.error {
		font-size: 0.8rem;
		color: var(--color-error);
	}

	/* Buttons */
	.btn-primary {
		padding: 10px 24px;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-md);
		color: white;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
		transition: opacity 0.15s;
	}

	.btn-primary:hover:not(:disabled) {
		opacity: 0.9;
	}

	.btn-primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-ghost {
		padding: 10px 24px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text);
		font-size: 0.9rem;
		cursor: pointer;
		transition: background 0.15s;
	}

	.btn-ghost:hover {
		background: var(--color-surface);
	}

	.nav-row {
		display: flex;
		justify-content: space-between;
		gap: var(--space-md);
		margin-top: var(--space-xs);
	}

	.nav-row .btn-primary {
		margin-left: auto;
	}

	/* Leaflet DivIcon marker — :global() because Leaflet injects these outside Svelte scope */
	:global(.setup-marker) {
		background: none !important;
		border: none !important;
	}

	:global(.setup-marker-pin) {
		width: 24px;
		height: 24px;
		background: var(--color-accent, #e63946);
		border: 3px solid #fff;
		border-radius: 50% 50% 50% 0;
		transform: rotate(-45deg);
		box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
		position: absolute;
		top: 0;
		left: 0;
	}

	:global(.setup-marker-shadow) {
		width: 14px;
		height: 4px;
		background: rgba(0, 0, 0, 0.25);
		border-radius: 50%;
		position: absolute;
		bottom: -2px;
		left: 5px;
	}
</style>

