<script lang="ts">
	import {
		mapSettings, updateMapSetting,
		AGE_FILTER_LABELS, TRACK_DURATION_LABELS, WX_MAP_MODE_LABELS, SAG_LEG_LINE_LABELS,
		sagFocusPresetOn, toggleSagFocusPreset,
		type StationAgeFilter, type TrackDuration, type SagLegLines
	} from '$lib/stores/mapSettings';
	import { rideMode, sagBoard, rideSagSummary } from '$lib/stores/ride';
	import type { WxMapMode } from '$lib/types';

	let {
		filteredCount = 0,
		totalCount = 0,
		hasActiveNet = false,
		rosterCount = 0,
		nextStopEnabled = true,
		hasCourse = false,
		onNextStopToggle,
	}: {
		filteredCount?: number;
		totalCount?: number;
		hasActiveNet?: boolean;
		rosterCount?: number;
		/** Whether the along-course "Next stop" readout is shown on the map. */
		nextStopEnabled?: boolean;
		/** A route-category annotation is loaded; without one the readout has nothing to measure. */
		hasCourse?: boolean;
		onNextStopToggle?: () => void;
	} = $props();

	let open = $state(false);
	let fabEl = $state<HTMLButtonElement>();
	let popEl = $state<HTMLDivElement>();

	// The roster filter needs a net with someone on it; without that the toggle
	// would silently blank the map, so it is disabled and says why.
	let rosterReason = $derived(
		!hasActiveNet ? 'No active net' : rosterCount === 0 ? 'No roster stations on the map' : ''
	);
	let rosterDisabled = $derived(rosterReason !== '');
	let rosterActive = $derived($mapSettings.showRosterOnly && !rosterDisabled);

	// Without a course line there is nothing to measure along, so the toggle is
	// disabled and says why rather than silently doing nothing.
	let nextStopReason = $derived(
		hasCourse ? '' : 'No course loaded — import a GPX or KML in Annotations.'
	);
	let nextStopDisabled = $derived(nextStopReason !== '');

	let hasNonDefault = $derived(
		$mapSettings.stationAgeFilter !== 'all' ||
		!$mapSettings.showTracks ||
		!$mapSettings.showDRCones ||
		$mapSettings.showCallsigns ||
		$mapSettings.showRosterOnly ||
		$mapSettings.showWeatherOverlay ||
		$mapSettings.showDFOverlay ||
		!nextStopEnabled ||
		$mapSettings.trackDuration !== 'all' ||
		$mapSettings.showWxAlerts !== 'watches' ||
		// SAG defaults to ON, so a FALSE here is the non-default worth warning
		// about — the indicator dot's job is to say "the map is hiding
		// something", and a hidden SAG layer is the worst case of that.
		($mapSettings.showSagOverlay === false && $rideMode) ||
		($mapSettings.sagLegLines !== 'selected' && $rideMode)
	);

	// The whole section exists only for a bike-ride net that actually has a SAG
	// board. A search-and-rescue net never sees a SAG control.
	let showSagSection = $derived($rideMode && !!$sagBoard);

	/**
	 * The panel is a `popover`, so it renders in the browser's TOP LAYER.
	 *
	 * It used to be an ordinary absolutely-positioned child at the same
	 * z-index as the GPS button, GPS pill and next-stop pill, and later in the
	 * DOM than none of them — so it opened BEHIND all three. No z-index can
	 * win that reliably against controls owned by other components; the top
	 * layer is above every one of them by construction, and brings Esc and
	 * click-outside dismissal with it.
	 *
	 * Where it opens: BESIDE the map HUD column when the window has room, so
	 * the SAG dock, the GPS chip and the next-stop readout stay readable while
	 * layers are being changed (toggling "Next stop readout" or "SAG dock"
	 * shows its effect instead of happening underneath the panel). On a
	 * phone there is no room beside anything, so it drops under its button.
	 * Either way it is clamped to the viewport and scrolls inside.
	 */
	function place() {
		if (!fabEl || !popEl) return;
		const r = fabEl.getBoundingClientRect();
		const gap = 8;
		const width = Math.min(280, window.innerWidth - 2 * gap);
		const column = fabEl.closest('.map-hud')?.getBoundingClientRect();
		const beside = column && column.right + gap + width + gap <= window.innerWidth - 48;
		const left = beside
			? column.right + gap
			: Math.max(gap, Math.min(r.left, window.innerWidth - width - gap));
		const top = beside ? r.top : r.bottom + gap;
		popEl.style.left = `${left}px`;
		popEl.style.top = `${top}px`;
		popEl.style.width = `${width}px`;
		// Stop above whatever owns the bottom edge — the phone's bottom sheet or
		// the desktop ride strip — rather than running over its controls.
		let floor = window.innerHeight;
		for (const sel of ['.bottom-sheet', '.ride-strip']) {
			const el = document.querySelector(sel);
			const t = el?.getBoundingClientRect().top;
			if (t != null && t > top && t < floor && el!.getClientRects().length) floor = t;
		}
		popEl.style.maxHeight = `${Math.max(160, floor - top - gap)}px`;
	}

	function handleToggle(e: ToggleEvent) {
		open = e.newState === 'open';
		if (open) place();
	}

	$effect(() => {
		if (!open) return;
		window.addEventListener('resize', place);
		return () => window.removeEventListener('resize', place);
	});
</script>

<div class="map-palette-wrapper">
	<button
		bind:this={fabEl}
		class="map-hud-btn map-palette-fab"
		class:map-palette-fab--open={open}
		popovertarget="map-layers-popover"
		aria-expanded={open}
		aria-label="Map layers and filters"
		title="Map layers & filters"
	>
		<svg width="20" height="20" viewBox="0 0 20 20" fill="none">
			<path d="M10 2L2 6l8 4 8-4-8-4z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
			<path d="M2 10l8 4 8-4" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
			<path d="M2 14l8 4 8-4" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
		</svg>
		{#if hasNonDefault}
			<span class="indicator-dot"></span>
		{/if}
	</button>

	<div
		bind:this={popEl}
		id="map-layers-popover"
		class="map-palette-popover"
		popover="auto"
		role="dialog"
		aria-label="Map layers"
		ontoggle={handleToggle}
	>
			<div class="palette-header">
				<span class="palette-title">Map Layers</span>
				<button class="palette-close" onclick={() => popEl?.hidePopover()} aria-label="Close">
					<svg width="14" height="14" viewBox="0 0 14 14" fill="none">
						<path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					</svg>
				</button>
			</div>

			<div class="palette-section">
				<div class="palette-row">
					<label class="palette-label" for="age-filter">Station age</label>
					<select
						id="age-filter"
						class="palette-select"
						value={$mapSettings.stationAgeFilter}
						disabled={rosterActive}
						aria-describedby="age-filter-note"
						onchange={(e) => updateMapSetting('stationAgeFilter', (e.target as HTMLSelectElement).value as StationAgeFilter)}
					>
						{#each Object.entries(AGE_FILTER_LABELS) as [value, label]}
							<option {value}>{label}</option>
						{/each}
					</select>
				</div>
				{#if rosterActive}
					<div class="palette-info" id="age-filter-note">
						Roster only overrides the age filter
					</div>
				{:else if $mapSettings.stationAgeFilter !== 'all'}
					<div class="palette-info" id="age-filter-note">
						Showing {filteredCount} of {totalCount} stations
					</div>
				{/if}
			</div>

			<div class="palette-divider"></div>

			<div class="palette-section">
				<div class="palette-row">
					<label class="palette-checkbox">
						<input
							type="checkbox"
							checked={$mapSettings.showTracks}
							onchange={() => updateMapSetting('showTracks', !$mapSettings.showTracks)}
						/>
						Tracks
					</label>
					<select
						class="palette-select"
						value={$mapSettings.trackDuration}
						disabled={!$mapSettings.showTracks}
						onchange={(e) => updateMapSetting('trackDuration', (e.target as HTMLSelectElement).value as TrackDuration)}
					>
						{#each Object.entries(TRACK_DURATION_LABELS) as [value, label]}
							<option {value}>{label}</option>
						{/each}
					</select>
				</div>
				<div class="palette-row">
					<label class="palette-checkbox">
						<input
							type="checkbox"
							checked={$mapSettings.showDRCones}
							onchange={() => updateMapSetting('showDRCones', !$mapSettings.showDRCones)}
						/>
						DR cones
					</label>
				</div>
				<div class="palette-row">
					<label class="palette-checkbox">
						<input
							type="checkbox"
							checked={$mapSettings.showCallsigns}
							onchange={() => updateMapSetting('showCallsigns', !$mapSettings.showCallsigns)}
						/>
						Call signs
					</label>
				</div>
				<div class="palette-row">
					<label class="palette-checkbox" class:is-disabled={rosterDisabled}>
						<input
							id="roster-only"
							type="checkbox"
							checked={$mapSettings.showRosterOnly}
							disabled={rosterDisabled}
							aria-describedby="roster-only-note"
							onchange={() => updateMapSetting('showRosterOnly', !$mapSettings.showRosterOnly)}
						/>
						Roster only
					</label>
				</div>
				<div class="palette-info" id="roster-only-note">
					{#if rosterDisabled}
						{rosterReason} — roster filter unavailable
					{:else if rosterActive}
						Showing {filteredCount} of {totalCount} stations
					{:else}
						Hides stations not checked into the net
					{/if}
				</div>
			</div>

			<div class="palette-divider"></div>

			<div class="palette-section">
				<div class="palette-row">
					<label class="palette-checkbox">
						<input
							type="checkbox"
							checked={$mapSettings.showWeatherOverlay}
							onchange={() => updateMapSetting('showWeatherOverlay', !$mapSettings.showWeatherOverlay)}
						/>
						Weather overlay
					</label>
				</div>
				<div class="palette-row">
					<label class="palette-checkbox">
						<input
							type="checkbox"
							checked={$mapSettings.showDFOverlay}
							onchange={() => updateMapSetting('showDFOverlay', !$mapSettings.showDFOverlay)}
						/>
						DF overlay
					</label>
				</div>
				<div class="palette-row">
					<label class="palette-label" for="wx-alerts-mode">NWS alerts</label>
					<select
						id="wx-alerts-mode"
						class="palette-select"
						value={$mapSettings.showWxAlerts}
						onchange={(e) => updateMapSetting('showWxAlerts', (e.target as HTMLSelectElement).value as WxMapMode)}
					>
						{#each Object.entries(WX_MAP_MODE_LABELS) as [value, label]}
							<option {value}>{label}</option>
						{/each}
					</select>
				</div>
				<div class="palette-info">Map only — never changes notifications</div>
				<div class="palette-row">
					<label class="palette-checkbox" class:is-disabled={nextStopDisabled}>
						<input
							id="next-stop-readout"
							type="checkbox"
							checked={nextStopEnabled && !nextStopDisabled}
							disabled={nextStopDisabled}
							aria-describedby="next-stop-note"
							onchange={() => onNextStopToggle?.()}
						/>
						Next stop readout
					</label>
				</div>
				<div class="palette-info" id="next-stop-note">
					{#if nextStopDisabled}
						{nextStopReason}
					{:else}
						Distance to the next stop along the course
					{/if}
				</div>

			</div>

			{#if showSagSection}
				<div class="palette-divider"></div>
				<div class="palette-section">
					<div class="palette-heading">SAG</div>
					<div class="palette-row">
						<label class="palette-checkbox">
							<input
								type="checkbox"
								checked={$mapSettings.showSagOverlay}
								onchange={(e) => updateMapSetting('showSagOverlay', (e.target as HTMLInputElement).checked)}
							/>
							SAG overlay
						</label>
						<span class="palette-count">{$rideSagSummary.open} open</span>
					</div>
					<div class="palette-row">
						<label class="palette-label" for="sag-leg-lines">Leg lines</label>
						<select
							id="sag-leg-lines"
							class="palette-select"
							disabled={!$mapSettings.showSagOverlay}
							value={$mapSettings.sagLegLines}
							onchange={(e) => updateMapSetting('sagLegLines', (e.target as HTMLSelectElement).value as SagLegLines)}
						>
							{#each Object.entries(SAG_LEG_LINE_LABELS) as [value, label]}
								<option {value}>{label}</option>
							{/each}
						</select>
					</div>
					<div class="palette-info">
						Every leg at once is unreadable on a busy ride; "Selected" draws only the
						request you are looking at.
					</div>
					<div class="palette-row">
						<label class="palette-checkbox">
							<input
								type="checkbox"
								checked={$mapSettings.showSagDock}
								disabled={!$mapSettings.showSagOverlay}
								onchange={(e) => updateMapSetting('showSagDock', (e.target as HTMLInputElement).checked)}
							/>
							SAG dock
						</label>
					</div>
					<div class="palette-info">
						Hiding the dock keeps the pins. Requests that cannot be placed on the
						map live only in the dock, so they go with it.
					</div>
					<div class="palette-row">
						<button
							class="palette-preset"
							class:palette-preset--on={$sagFocusPresetOn}
							aria-pressed={$sagFocusPresetOn}
							onclick={toggleSagFocusPreset}
						>
							{$sagFocusPresetOn ? 'Restore my layers' : 'SAG focus'}
						</button>
					</div>
					<div class="palette-info">
						Hides tracks, DR cones, callsigns, weather and DF, and shows only the
						last hour of stations. Press it again to put every one of them back.
					</div>
				</div>
			{/if}
	</div>
</div>

<style>
	.map-palette-wrapper {
		display: contents;
	}

	/* Size, surface, hover and focus ring come from .map-hud-btn (app.css). */
	.map-palette-fab {
		position: relative;
	}

	.map-palette-fab--open {
		border-color: var(--color-text-muted);
		background: color-mix(in srgb, var(--color-text) 12%, var(--color-surface));
	}

	.indicator-dot {
		position: absolute;
		top: 4px;
		right: 4px;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--color-primary, #6366f1);
		border: 1.5px solid var(--color-surface, #1a1a2e);
	}

	/* A top-layer popover: undo the UA's centred-dialog defaults, then place
	   it with the inline top/left/width/max-height place() computes. */
	.map-palette-popover {
		position: fixed;
		inset: auto;
		margin: 0;
		overflow-y: auto;
		overscroll-behavior: contain;
		background: var(--color-surface, #1a1a2e);
		color: var(--color-text);
		border: 1px solid var(--color-primary, #6366f1);
		border-radius: var(--radius-md, 8px);
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(0,0,0,0.45));
		padding: 0;
	}

	.map-palette-popover .palette-header {
		position: sticky;
		top: 0;
		z-index: 1;
		background: var(--color-surface, #1a1a2e);
	}

	.palette-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0.75rem;
		border-bottom: 1px solid rgba(255,255,255,0.08);
	}

	.palette-title {
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--color-text, #eee);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.palette-close {
		background: none;
		border: none;
		color: var(--color-text-muted, #888);
		cursor: pointer;
		padding: 2px;
		border-radius: 4px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.palette-close:hover {
		color: var(--color-text, #eee);
		background: rgba(255,255,255,0.08);
	}

	.palette-section {
		padding: 0.5rem 0.75rem;
	}

	.palette-heading {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--color-text-muted);
		padding-bottom: 0.35rem;
	}

	.palette-count {
		margin-left: auto;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
	}

	.palette-preset {
		flex: 1;
		min-height: 32px;
		padding: 0 var(--space-sm);
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--font-sm, 0.8125rem);
		font-weight: 600;
		cursor: pointer;
	}

	.palette-preset--on {
		border-color: var(--color-sag-unit);
		color: var(--color-sag-unit);
	}

	.palette-divider {
		height: 1px;
		background: rgba(255,255,255,0.06);
	}

	.palette-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		min-height: 28px;
	}

	.palette-row + .palette-row {
		margin-top: 0.35rem;
	}

	.palette-label {
		font-size: 0.8rem;
		color: var(--color-text, #eee);
	}

	.palette-select {
		flex-shrink: 0;
		padding: 3px 6px;
		font-size: 0.75rem;
		border-radius: 4px;
		border: 1px solid rgba(255,255,255,0.15);
		background: rgba(255,255,255,0.06);
		color: var(--color-text, #eee);
		cursor: pointer;
		outline: none;
	}

	.palette-select:focus {
		border-color: var(--color-primary, #6366f1);
	}

	.palette-select:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.palette-checkbox {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.8rem;
		color: var(--color-text, #eee);
		cursor: pointer;
	}

	.palette-checkbox.is-disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.palette-checkbox input[type="checkbox"]:disabled {
		cursor: not-allowed;
	}

	.palette-checkbox input[type="checkbox"] {
		width: 14px;
		height: 14px;
		accent-color: var(--color-primary, #6366f1);
		cursor: pointer;
	}

	.palette-info {
		font-size: 0.7rem;
		color: var(--color-text-muted, #888);
		margin-top: 0.25rem;
	}
</style>
