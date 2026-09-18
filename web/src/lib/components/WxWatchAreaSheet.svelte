<script lang="ts">
	import { api } from '$lib/api';
	import { showToast } from '$lib/stores/toast';
	import { showFootprintPreview } from '$lib/stores/wxAlerts';
	import { FLOOR_TEXT } from '$lib/wxAlertMeta';
	import WxTierGlyph from './WxTierGlyph.svelte';
	import type { WxNetWatch, WxNetWatchZones, WxFootprintSummary, WxEventType, WxZoneRef } from '$lib/types';

	let { netId, onClose }: { netId: string; onClose: () => void } = $props();

	let loading = $state(true);
	let saving = $state(false);
	let watch = $state<WxNetWatch | null>(null);
	let zones = $state<WxNetWatchZones | null>(null);
	let footprint = $state<WxFootprintSummary | null>(null);
	let eventTypes = $state<WxEventType[]>([]);

	let bufferMiles = $state(10);
	let extraZones = $state<string[]>([]);
	let interruptEvents = $state<string[]>([]);
	let muteAdvisories = $state(false);

	let zoneQuery = $state('');
	let zoneSearchResults = $state<WxZoneRef[]>([]);
	let eventQuery = $state('');

	$effect(() => {
		let cancelled = false;
		(async () => {
			try {
				const [w, z, fp, et] = await Promise.all([
					api.getNetWxWatch(netId),
					api.netWxWatchZones(netId),
					api.wxFootprint(),
					api.wxEventTypes()
				]);
				if (cancelled) return;
				watch = w;
				zones = z;
				footprint = fp.summary;
				eventTypes = et;
				bufferMiles = w.bufferMiles || w.effective.bufferMiles || 10;
				extraZones = [...w.extraZones];
				interruptEvents = w.interruptCustom ? [...w.interruptEvents] : [...w.effective.interruptEvents];
				muteAdvisories = w.muteAdvisories;
			} catch {
				showToast('Could not load the weather watch area.', 'error');
			} finally {
				if (!cancelled) loading = false;
			}
		})();
		return () => {
			cancelled = true;
		};
	});

	let warningEventTypes = $derived(eventTypes.filter((e) => e.tier === 'warning'));
	let filteredEventOptions = $derived(
		warningEventTypes.filter((e) => !interruptEvents.includes(e.event) && e.event.toLowerCase().includes(eventQuery.toLowerCase()))
	);
	let neighborOptions = $derived((zones?.neighbors ?? []).filter((z: WxZoneRef) => !extraZones.includes(z.ugc)));

	function zoneName(ugc: string): string {
		const inNeighbors = (zones?.neighbors ?? []).find((z: WxZoneRef) => z.ugc === ugc)?.name;
		const inFootprint = (footprint?.zones ?? []).find((z: WxZoneRef) => z.ugc === ugc)?.name;
		return inNeighbors ?? inFootprint ?? ugc;
	}

	function toggleExtraZone(ugc: string): void {
		extraZones = extraZones.includes(ugc) ? extraZones.filter((z) => z !== ugc) : [...extraZones, ugc];
	}

	function toggleInterruptEvent(event: string): void {
		interruptEvents = interruptEvents.filter((e) => e !== event);
	}

	function addEventFromCombobox(event: string): void {
		if (!interruptEvents.includes(event)) interruptEvents = [...interruptEvents, event];
		eventQuery = '';
	}

	function resetInterruptEvents(): void {
		interruptEvents = [...(watch?.effective.interruptEvents ?? [])];
	}

	let zoneSearchTimer: ReturnType<typeof setTimeout> | null = null;

	function handleZoneQuery(q: string): void {
		zoneQuery = q;
		if (zoneSearchTimer) clearTimeout(zoneSearchTimer);
		if (!q.trim()) {
			zoneSearchResults = [];
			return;
		}
		const state = (footprint?.zones ?? [])[0]?.state || (zones?.resolved ?? [])[0]?.state || '';
		if (!state) return;
		zoneSearchTimer = setTimeout(async () => {
			try {
				zoneSearchResults = (await api.wxZoneSearch(q, state)).filter((z) => !extraZones.includes(z.ugc));
			} catch {
				zoneSearchResults = [];
			}
		}, 300);
	}

	function addSearchedZone(ugc: string): void {
		toggleExtraZone(ugc);
		zoneSearchResults = zoneSearchResults.filter((z) => z.ugc !== ugc);
	}

	async function handleSave(): Promise<void> {
		if (!watch) return;
		saving = true;
		try {
			const defaults = [...(watch.effective.interruptEvents ?? [])].sort();
			const custom = JSON.stringify([...interruptEvents].sort()) !== JSON.stringify(defaults);
			await api.updateNetWxWatch(netId, {
				netId,
				bufferMiles,
				extraZones,
				muteAdvisories,
				interruptCustom: custom,
				interruptEvents,
				effective: watch.effective
			});
			showToast('Watch area saved', 'success');
			onClose();
		} catch {
			showToast('Could not save the watch area.', 'error');
		} finally {
			saving = false;
		}
	}
</script>

<div class="wx-watch-sheet">
	<div class="wx-watch-header">
		<button class="wx-watch-back" onclick={onClose}>‹ Net Control</button>
		<h2 class="wx-watch-h2">Weather watch area</h2>
	</div>

	{#if loading}
		<p class="wx-watch-loading">Loading…</p>
	{:else}
		<div class="wx-watch-body">
			<section class="wx-watch-section">
				<h3 class="wx-watch-title">Watch footprint (derived automatically)</h3>
				<div class="wx-watch-readonly">
					<p>Course {footprint?.routeMiles ?? 0} mi route</p>
					<p>Locations {footprint?.locationCount ?? 0}</p>
					<p>Positions {footprint?.rosterPositions ?? 0} roster · {footprint?.trackedPositions ?? 0} tracked</p>
					<p>
						Own station {footprint?.ownStation === 'gps'
							? `GPS fix ${footprint.ownStationAgeSec}s ago`
							: footprint?.ownStation === 'config'
								? 'config position'
								: 'none'}
					</p>
					<div class="wx-watch-divider"></div>
					<p class="wx-watch-subtle">Resolved zones</p>
					<div class="wx-watch-chips">
						{#each footprint?.zones ?? [] as z (z.ugc)}
							<span class="wx-chip">{z.name || z.ugc} ({z.ugc})</span>
						{/each}
					</div>
				</div>
				<button class="wx-watch-btn" onclick={() => showFootprintPreview()}>Show on map · 10 s</button>
			</section>

			<section class="wx-watch-section">
				<h3 class="wx-watch-title">Buffer around course &amp; positions</h3>
				<div class="wx-watch-slider-row">
					<input type="range" min="2" max="50" step="1" bind:value={bufferMiles} />
					<span class="wx-watch-slider-value">{bufferMiles} mi</span>
				</div>
				<p class="wx-watch-caption">Alerts touching this buffer are "in watch area"; within 2× are "nearby".</p>
			</section>

			<section class="wx-watch-section">
				<h3 class="wx-watch-title">Extra zones</h3>
				<div class="wx-watch-list">
					{#each extraZones as ugc (ugc)}
						<label class="wx-watch-check">
							<input type="checkbox" checked onclick={() => toggleExtraZone(ugc)} />
							{zoneName(ugc)} ({ugc})
						</label>
					{/each}
					{#each neighborOptions as z (z.ugc)}
						<label class="wx-watch-check">
							<input type="checkbox" onclick={() => toggleExtraZone(z.ugc)} />
							{z.name} ({z.ugc}) · neighbor
						</label>
					{/each}
				</div>
				<input
					class="wx-watch-search"
					placeholder="Search zones…"
					value={zoneQuery}
					oninput={(e) => handleZoneQuery((e.target as HTMLInputElement).value)}
				/>
				{#if zoneSearchResults.length}
					<div class="wx-watch-list">
						{#each zoneSearchResults as z (z.ugc)}
							<button class="wx-watch-addable" onclick={() => addSearchedZone(z.ugc)}>+ {z.name} ({z.ugc})</button>
						{/each}
					</div>
				{/if}
			</section>

			<section class="wx-watch-section">
				<h3 class="wx-watch-title">Interrupt events</h3>
				<div class="wx-watch-list">
					{#each interruptEvents as event (event)}
						<label class="wx-watch-check">
							<input type="checkbox" checked onclick={() => toggleInterruptEvent(event)} />
							<WxTierGlyph tier="warning" size={12} />
							{event}
						</label>
					{/each}
				</div>
				<input
					class="wx-watch-search"
					placeholder="+ add event…"
					value={eventQuery}
					oninput={(e) => (eventQuery = (e.target as HTMLInputElement).value)}
				/>
				{#if eventQuery && filteredEventOptions.length}
					<div class="wx-watch-list">
						{#each filteredEventOptions.slice(0, 8) as e (e.event)}
							<button class="wx-watch-addable" onclick={() => addEventFromCombobox(e.event)}>+ {e.event}</button>
						{/each}
					</div>
				{/if}
				<p class="wx-watch-caption">
					Alerts on this list interrupt every operator with a full-screen banner. Removing an event lowers it to a toast. {FLOOR_TEXT}
				</p>
				<button class="wx-watch-link" onclick={resetInterruptEvents}>Reset to defaults</button>
			</section>

			<section class="wx-watch-section">
				<h3 class="wx-watch-title">Noise floor for this net</h3>
				<label class="wx-toggle-row">
					<span class="wx-toggle-copy">
						<span class="wx-toggle-label">Mute advisories &amp; statements</span>
						<span class="wx-toggle-caption">Warnings can never be muted.</span>
					</span>
					<input type="checkbox" bind:checked={muteAdvisories} />
				</label>
			</section>
		</div>

		<div class="wx-watch-footer">
			<button class="wx-watch-btn" onclick={onClose} disabled={saving}>Cancel</button>
			<button class="wx-watch-btn wx-watch-btn-accent" onclick={handleSave} disabled={saving}>Save</button>
		</div>
	{/if}
</div>

<style>
	.wx-watch-sheet {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}

	.wx-watch-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.wx-watch-back {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		cursor: pointer;
	}

	.wx-watch-h2 {
		font-size: 0.9rem;
		font-weight: 700;
	}

	.wx-watch-loading {
		padding: var(--space-md);
		color: var(--color-text-muted);
	}

	.wx-watch-body {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-sm) var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.wx-watch-section {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.wx-watch-title {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		color: var(--color-text-muted);
	}

	.wx-watch-readonly {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		padding: var(--space-sm);
		font-size: 0.8rem;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.wx-watch-divider {
		height: 1px;
		background: var(--color-primary);
		margin: 4px 0;
	}

	.wx-watch-subtle {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-watch-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin-top: 4px;
	}

	.wx-chip {
		display: inline-flex;
		align-items: center;
		font-size: 0.65rem;
		font-weight: 600;
		border: 1px solid var(--color-text-muted);
		color: var(--color-text-muted);
		border-radius: 8px;
		padding: 2px 8px;
	}

	.wx-watch-btn {
		align-self: flex-start;
		min-height: 40px;
		padding: 0 var(--space-md);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-watch-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.wx-watch-btn-accent {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}

	.wx-watch-slider-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.wx-watch-slider-row input[type='range'] {
		flex: 1;
	}

	.wx-watch-slider-value {
		font-variant-numeric: tabular-nums;
		font-size: 0.85rem;
		font-weight: 600;
		min-width: 48px;
		text-align: right;
	}

	.wx-watch-caption {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		line-height: 1.4;
	}

	.wx-watch-list {
		display: flex;
		flex-direction: column;
		gap: 2px;
		max-height: 160px;
		overflow-y: auto;
	}

	.wx-watch-check {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		min-height: 36px;
		font-size: 0.8rem;
	}

	.wx-watch-addable {
		display: flex;
		align-items: center;
		min-height: 36px;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
	}

	.wx-watch-search {
		min-height: 40px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
	}

	.wx-watch-link {
		align-self: flex-start;
		background: none;
		border: none;
		padding: 0;
		color: var(--color-accent);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-toggle-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		min-height: 44px;
		cursor: pointer;
	}

	.wx-toggle-copy {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.wx-toggle-label {
		font-size: 0.85rem;
		font-weight: 600;
	}

	.wx-toggle-caption {
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.wx-toggle-row input[type='checkbox'] {
		flex-shrink: 0;
		width: 36px;
		height: 20px;
		appearance: none;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.15);
		border-radius: 10px;
		cursor: pointer;
		position: relative;
	}

	.wx-toggle-row input[type='checkbox']::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 14px;
		height: 14px;
		background: var(--color-text-muted);
		border-radius: 50%;
		transition: transform var(--duration-fast), background var(--duration-fast);
	}

	.wx-toggle-row input[type='checkbox']:checked {
		background: color-mix(in srgb, var(--color-success) 20%, transparent);
		border-color: color-mix(in srgb, var(--color-success) 40%, transparent);
	}

	.wx-toggle-row input[type='checkbox']:checked::after {
		transform: translateX(16px);
		background: var(--color-success);
	}

	.wx-watch-footer {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		border-top: 1px solid var(--color-primary);
	}
</style>
