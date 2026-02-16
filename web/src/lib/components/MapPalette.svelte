<script lang="ts">
	import {
		mapSettings, updateMapSetting,
		AGE_FILTER_LABELS, TRACK_DURATION_LABELS,
		type StationAgeFilter, type TrackDuration
	} from '$lib/stores/mapSettings';

	let {
		filteredCount = 0,
		totalCount = 0,
	}: {
		filteredCount?: number;
		totalCount?: number;
	} = $props();

	let open = $state(false);

	let hasNonDefault = $derived(
		$mapSettings.stationAgeFilter !== 'all' ||
		!$mapSettings.showTracks ||
		!$mapSettings.showDRCones ||
		$mapSettings.showWeatherOverlay ||
		$mapSettings.showDFOverlay ||
		$mapSettings.trackDuration !== 'all'
	);

	function toggle() {
		open = !open;
	}

	function handleBlur() {
		setTimeout(() => {
			const popover = document.querySelector('.map-palette-popover');
			if (popover && !popover.contains(document.activeElement)) {
				open = false;
			}
		}, 150);
	}
</script>

<div class="map-palette-wrapper">
	<button
		class="map-palette-fab"
		onclick={toggle}
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

	{#if open}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="map-palette-popover" onblur={handleBlur} tabindex="-1">
			<div class="palette-header">
				<span class="palette-title">Map Layers</span>
				<button class="palette-close" onclick={() => open = false} aria-label="Close">
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
						onchange={(e) => updateMapSetting('stationAgeFilter', (e.target as HTMLSelectElement).value as StationAgeFilter)}
					>
						{#each Object.entries(AGE_FILTER_LABELS) as [value, label]}
							<option {value}>{label}</option>
						{/each}
					</select>
				</div>
				{#if $mapSettings.stationAgeFilter !== 'all'}
					<div class="palette-info">
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
			</div>
		</div>
	{/if}
</div>

<style>
	.map-palette-wrapper {
		position: absolute;
		bottom: 30px;
		left: 10px;
		z-index: var(--z-toolbar);
	}

	.map-palette-fab {
		width: 40px;
		height: 40px;
		border-radius: 8px;
		border: 1px solid var(--color-border, rgba(255,255,255,0.12));
		background: var(--color-surface, #1a1a2e);
		color: var(--color-text, #eee);
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		box-shadow: var(--shadow-md, 0 2px 8px rgba(0,0,0,0.3));
		transition: background 0.15s, border-color 0.15s;
		position: relative;
	}

	.map-palette-fab:hover {
		background: var(--color-surface-hover, #252540);
		border-color: var(--color-primary, #6366f1);
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

	.map-palette-popover {
		position: absolute;
		bottom: 48px;
		left: 0;
		width: 260px;
		background: var(--color-surface, #1a1a2e);
		border: 1px solid var(--color-primary, #6366f1);
		border-radius: var(--radius-md, 8px);
		box-shadow: var(--shadow-md, 0 4px 16px rgba(0,0,0,0.4));
		padding: 0;
		outline: none;
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
