<script lang="ts">
	import type { WxAlert } from '$lib/types';
	import { api } from '$lib/api';
	import {
		wxClock,
		wxMinute,
		wxIsAcked,
		wxIsNcs,
		wxAcked,
		wxMuted,
		ackAlert,
		ackAlertForNet,
		muteAlert,
		unmuteAlert,
		showAlertOnMap
	} from '$lib/stores/wxAlerts';
	import { showToast } from '$lib/stores/toast';
	import { tierMeta, zoneLabel, toastLine } from '$lib/wxAlertMeta';
	import { clock, clockWithSeconds, countdown, countdownTone, progress } from '$lib/wxAlertTime';
	import WxTierGlyph from './WxTierGlyph.svelte';
	import WxRelayComposer from './WxRelayComposer.svelte';

	let { alert, onBack, onFlyTo }: { alert: WxAlert; onBack: () => void; onFlyTo?: (lat: number, lon: number) => void } = $props();

	let meta = $derived(tierMeta[alert.tier]);
	let acked = $derived($wxIsAcked(alert));
	let ackedAt = $derived($wxAcked.get(alert.id));
	let muted = $derived($wxMuted.has(alert.id));
	let isEnded = $derived(alert.state !== 'active');

	// The countdown ticks every second under 5 minutes, once a minute otherwise.
	let clockNow = $derived(countdown(alert.endsAt, $wxClock).msLeft <= 5 * 60000 ? $wxClock : $wxMinute * 60000);
	let cd = $derived(countdown(alert.endsAt, clockNow));
	let tone = $derived(countdownTone(alert.endsAt, clockNow));
	let bar = $derived(progress(alert.effective, alert.endsAt, clockNow));

	let showMoreDescription = $state(false);
	let menuOpen = $state(false);
	let menuWrapEl = $state<HTMLElement | null>(null);
	let relayOpen = $state(false);
	let history = $state<WxAlert[] | null>(null);
	let historyLoading = $state(false);
	let showingPrevious = $state<WxAlert | null>(null);

	let displayed = $derived(showingPrevious ?? alert);

	// Close the footer "more actions" menu on outside click, and on Escape —
	// captured before SidePanel's bubble-phase Escape handler sees it, so
	// Escape closes the menu first and reaches the panel on the next press
	// instead of being swallowed by data-blocks-escape.
	$effect(() => {
		if (!menuOpen) return;
		const onDown = (e: PointerEvent) => {
			if (!menuWrapEl?.contains(e.target as Node)) menuOpen = false;
		};
		const onKey = (e: KeyboardEvent) => {
			if (e.key === 'Escape') {
				e.stopImmediatePropagation();
				menuOpen = false;
			}
		};
		window.addEventListener('pointerdown', onDown, true);
		window.addEventListener('keydown', onKey, true);
		return () => {
			window.removeEventListener('pointerdown', onDown, true);
			window.removeEventListener('keydown', onKey, true);
		};
	});

	async function loadHistory(): Promise<void> {
		if (history) {
			showingPrevious = history[0] ?? null;
			return;
		}
		historyLoading = true;
		try {
			const res = await api.wxAlert(alert.id);
			history = res.history;
			showingPrevious = res.history[0] ?? null;
		} catch {
			showToast('Could not load the earlier version.', 'error');
		} finally {
			historyLoading = false;
		}
	}

	function backToCurrent(): void {
		showingPrevious = null;
	}

	async function handleAcknowledge(): Promise<void> {
		await ackAlert(alert.id);
	}

	async function handleAckForNet(): Promise<void> {
		try {
			await ackAlertForNet(alert.id);
			showToast('Acknowledged for the net.', 'success');
		} catch {
			showToast('Could not acknowledge for the net.', 'error');
		}
	}

	function handleMuteToggle(): void {
		if (muted) unmuteAlert(alert.id);
		else muteAlert(alert.id);
		menuOpen = false;
	}

	async function handleCopySummary(): Promise<void> {
		menuOpen = false;
		try {
			await navigator.clipboard.writeText(toastLine(alert));
			showToast('Copied.', 'success', 2000);
		} catch {
			// Silent — clipboard access may be blocked.
		}
	}

	function openOnWeatherGov(): void {
		menuOpen = false;
		window.open(`https://alerts.weather.gov/search?id=${encodeURIComponent(alert.id)}`, '_blank', 'noopener');
	}
</script>

<div class="wx-detail">
	{#if showingPrevious}
		<div class="wx-detail-breadcrumb">
			<button class="wx-breadcrumb-link" onclick={backToCurrent}>Alerts</button>
			›
			<button class="wx-breadcrumb-link" onclick={backToCurrent}>{alert.event}</button>
			› earlier version
		</div>
	{/if}

	<div class="wx-detail-scroll">
		<div class="wx-detail-band" style="background: var({meta.colorVar})" class:wx-band-hatched={displayed.severity === 'Extreme' || displayed.severity === 'Severe'}></div>

		<div class="wx-detail-header">
			<button class="wx-back" onclick={onBack} aria-label="Back to alert list">
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
					<path d="M10 3L5 8l5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
				</svg>
				<span>Alerts</span>
			</button>
			<WxTierGlyph tier={displayed.tier} size={20} title={meta.ariaWord} />
			<h2 class="wx-detail-title">{displayed.event}</h2>
		</div>

		<div class="wx-detail-chips">
			<span class="wx-chip">{displayed.severity}</span>
			<span class="wx-chip">{displayed.certainty}</span>
			<span class="wx-chip">{displayed.urgency}</span>
			{#if displayed.geometrySource === 'zone'}<span class="wx-chip wx-chip-zone">ZONE</span>{/if}
			{#if displayed.status !== 'Actual'}<span class="wx-chip wx-chip-test">TEST</span>{/if}
		</div>

		<div class="wx-timing" aria-label="Ends {clock(displayed.endsAt)}">
			{#if isEnded}
				<div class="wx-timing-row">
					<span class="wx-timing-label">ENDED {clock(displayed.endedAt ?? displayed.endsAt)}</span>
				</div>
			{:else}
				<div class="wx-timing-row">
					<span class="wx-timing-label">ENDS {clock(displayed.endsAt)}</span>
					<time datetime={displayed.endsAt} class="wx-timing-countdown" class:soon={tone === 'soon'} aria-live="off">{cd.text}</time>
				</div>
			{/if}
			<div class="wx-timing-bar" class:saturate={isEnded}>
				<div class="wx-timing-fill" style="width: {bar * 100}%; background: var({meta.colorVar})"></div>
			</div>
			<div class="wx-timing-sub">issued {clock(displayed.sent)} · effective {clock(displayed.effective)}</div>
		</div>

		{#if displayed.instruction}
			<div class="wx-instruction" style="background: var({meta.softVar}); border-left-color: var({meta.colorVar})">
				<h3 class="wx-section-title" style="color: var({meta.colorVar})">Instruction</h3>
				<p class="wx-pre">{displayed.instruction}</p>
			</div>
		{/if}

		{#if displayed.affects.checkpoints.length || displayed.affects.locations.length || displayed.affects.stations.length}
			<div class="wx-section">
				<h3 class="wx-section-title">Affects (yours)</h3>
				{#if displayed.geometrySource === 'zone'}
					<p class="wx-honesty">
						Zone-based alert ({zoneLabel(displayed)}). NWS did not publish a polygon
						{#if displayed.zones.some((z) => !z.cached)}
							and the zone outline is not cached; highlighted where your course passes through the zone.
						{:else}
							; this is the NWS zone outline.
						{/if}
					</p>
				{/if}
				{#if displayed.affects.checkpoints.length}
					<div class="wx-affects-row">
						<span class="wx-affects-label">Checkpoints</span>
						<div class="wx-affects-chips">
							{#each displayed.affects.checkpoints as item (item.id)}
								<button class="wx-chip wx-chip-btn" onclick={() => onFlyTo?.(item.lat, item.lon)}>{item.shortName || item.label}</button>
							{/each}
						</div>
					</div>
				{/if}
				{#if displayed.affects.locations.length}
					<div class="wx-affects-row">
						<span class="wx-affects-label">Locations</span>
						<div class="wx-affects-chips">
							{#each displayed.affects.locations as item (item.id)}
								<button class="wx-chip wx-chip-btn" onclick={() => onFlyTo?.(item.lat, item.lon)}>{item.shortName || item.label}</button>
							{/each}
						</div>
					</div>
				{/if}
				{#if displayed.affects.stations.length}
					<div class="wx-affects-row">
						<span class="wx-affects-label">Stations</span>
						<div class="wx-affects-chips">
							{#each displayed.affects.stations as item (item.id)}
								<button class="wx-chip wx-chip-btn" onclick={() => onFlyTo?.(item.lat, item.lon)}>{item.shortName || item.label}</button>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{:else}
			<div class="wx-section">
				<h3 class="wx-section-title">Affects (yours)</h3>
				<p class="wx-empty-note">Nothing of yours is inside this alert.</p>
			</div>
		{/if}

		{#if displayed.headline}
			<div class="wx-section">
				<h3 class="wx-section-title">Headline</h3>
				<p class="wx-pre">{displayed.headline}</p>
			</div>
		{/if}

		<div class="wx-section">
			<h3 class="wx-section-title">Description</h3>
			<pre class="wx-desc" class:clamped={!showMoreDescription}>{displayed.description}</pre>
			<button class="wx-show-more" aria-expanded={showMoreDescription} onclick={() => (showMoreDescription = !showMoreDescription)}>
				{showMoreDescription ? 'Show less' : 'Show more'}
			</button>
		</div>

		<div class="wx-section">
			<h3 class="wx-section-title">Affected area (NWS)</h3>
			<p>{displayed.areaDesc}</p>
			{#if displayed.ugc.length}<p class="wx-ugc">{displayed.ugc.join(', ')}</p>{/if}
		</div>

		<div class="wx-section wx-source">
			<p>{displayed.senderName} ({displayed.senderId})</p>
			<p class="wx-source-sub">sent {clock(displayed.sent)} · fetched {clockWithSeconds(displayed.fetchedAt)}</p>
			{#if !showingPrevious && displayed.references.length > 0}
				<button class="wx-replaces" onclick={loadHistory} disabled={historyLoading}>
					↺ Replaces earlier alert ({clock(displayed.references[0].sent)})
				</button>
			{/if}
			{#if displayed.ackedForNet}
				<p class="wx-source-sub">Acked for net by {displayed.ackedForNet.callsign || displayed.ackedForNet.userName} · {clock(displayed.ackedForNet.at)}</p>
			{/if}
			{#if displayed.endedReason === 'dropped'}
				<p class="wx-source-sub">no longer listed by NWS</p>
			{:else if displayed.endedReason === 'clock'}
				<p class="wx-source-sub">expired by clock — no NWS confirmation</p>
			{/if}
		</div>
	</div>

	<div class="wx-detail-footer">
		<button class="wx-footer-btn" onclick={() => showAlertOnMap(alert.id)}>Show on map</button>
		{#if acked}
			<span class="wx-footer-acked">Acknowledged{ackedAt ? ` ${clock(ackedAt)}` : ''}</span>
		{:else}
			<button class="wx-footer-btn wx-footer-btn-accent" onclick={handleAcknowledge}>Acknowledge</button>
		{/if}
		{#if $wxIsNcs}
			{#if alert.ackedForNet}
				<span class="wx-footer-acked">Acked for net</span>
			{:else}
				<button class="wx-footer-btn" onclick={handleAckForNet}>Ack for net</button>
			{/if}
			<button class="wx-footer-btn" onclick={() => (relayOpen = true)}>Relay to net…</button>
		{/if}
		<div class="wx-footer-menu-wrap" bind:this={menuWrapEl}>
			<button class="wx-footer-btn wx-icon-btn" aria-haspopup="menu" aria-expanded={menuOpen} aria-label="More actions" onclick={() => (menuOpen = !menuOpen)}>
				⋯
			</button>
			{#if menuOpen}
				<div class="wx-footer-menu" role="menu" data-blocks-escape="true">
					{#if alert.notifyClass !== 'interrupt'}
						<button role="menuitem" class="wx-row-menu-item" onclick={handleMuteToggle}>{muted ? 'Unmute' : 'Mute'}</button>
					{/if}
					<button role="menuitem" class="wx-row-menu-item" onclick={handleCopySummary}>Copy summary</button>
					<button role="menuitem" class="wx-row-menu-item" onclick={openOnWeatherGov}>Open on weather.gov</button>
				</div>
			{/if}
		</div>
	</div>
</div>

{#if relayOpen}
	<WxRelayComposer {alert} onClose={() => (relayOpen = false)} />
{/if}

<style>
	.wx-detail {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}

	.wx-detail-breadcrumb {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: var(--space-xs) var(--space-md);
		font-size: 0.7rem;
		color: var(--color-text-muted);
		border-bottom: 1px solid var(--color-primary);
	}

	.wx-breadcrumb-link {
		background: none;
		border: none;
		padding: 0;
		color: var(--color-accent);
		font-size: inherit;
		cursor: pointer;
	}

	.wx-detail-scroll {
		flex: 1;
		overflow-y: auto;
		padding-bottom: var(--space-lg);
	}

	.wx-detail-band {
		height: 4px;
	}

	.wx-band-hatched {
		background-image: repeating-linear-gradient(45deg, rgba(255, 255, 255, 0.35) 0, rgba(255, 255, 255, 0.35) 2px, transparent 2px, transparent 6px);
	}

	.wx-detail-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-md);
	}

	.wx-back {
		display: inline-flex;
		align-items: center;
		gap: var(--space-xs);
		flex-shrink: 0;
		min-height: 44px;
		padding: 0 var(--space-sm) 0 var(--space-xs);
		margin-left: calc(-1 * var(--space-xs));
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-accent);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-back:hover,
	.wx-back:focus-visible {
		background: var(--color-primary);
		color: var(--color-text);
	}

	.wx-detail-title {
		flex: 1;
		min-width: 0;
		font-size: 1rem;
		font-weight: 700;
	}

	.wx-detail-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		padding: 0 var(--space-md) var(--space-sm);
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

	.wx-chip-zone,
	.wx-chip-test {
		border-color: var(--color-wx-watch);
		color: var(--color-wx-watch);
	}

	.wx-chip-btn {
		background: var(--color-bg);
		min-height: 32px;
		cursor: pointer;
	}

	.wx-chip-btn:hover,
	.wx-chip-btn:focus-visible {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.wx-timing {
		padding: var(--space-sm) var(--space-md) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.wx-timing-row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-sm);
	}

	.wx-timing-label {
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.wx-timing-countdown {
		font-size: 1.25rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.wx-timing-countdown.soon {
		color: var(--color-wx-watch);
	}

	.wx-timing-bar {
		margin-top: var(--space-xs);
		height: 6px;
		border-radius: var(--radius-full);
		background: var(--color-primary);
		overflow: hidden;
	}

	.wx-timing-bar.saturate {
		filter: saturate(0.4);
	}

	.wx-timing-fill {
		height: 100%;
	}

	.wx-timing-sub {
		margin-top: 4px;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-section {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.wx-section-title {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		color: var(--color-text-muted);
		margin-bottom: 4px;
	}

	.wx-honesty {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		margin-bottom: var(--space-xs);
	}

	.wx-affects-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
		margin-bottom: var(--space-xs);
	}

	.wx-affects-label {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-affects-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.wx-empty-note {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.wx-instruction {
		margin: var(--space-sm) var(--space-md);
		padding: var(--space-sm) var(--space-md);
		border-left: 3px solid;
		border-radius: var(--radius-sm);
	}

	.wx-pre {
		font: inherit;
		white-space: pre-line;
		font-size: 0.85rem;
	}

	.wx-desc {
		font: inherit;
		white-space: pre-wrap;
		font-size: 0.85rem;
		margin: 0;
	}

	.wx-desc.clamped {
		display: -webkit-box;
		-webkit-line-clamp: 4;
		line-clamp: 4;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.wx-show-more {
		margin-top: 4px;
		background: none;
		border: none;
		padding: 0;
		color: var(--color-accent);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-ugc {
		font-family: monospace;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-source {
		font-size: 0.8rem;
	}

	.wx-source-sub {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		margin-top: 2px;
	}

	.wx-replaces {
		margin-top: var(--space-xs);
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 6px 10px;
		color: var(--color-text);
		font-size: 0.75rem;
		cursor: pointer;
	}

	.wx-detail-footer {
		position: sticky;
		bottom: 0;
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
		padding: var(--space-sm) var(--space-md);
		background: var(--color-bg);
		border-top: 1px solid var(--color-primary);
	}

	.wx-footer-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-footer-btn:hover,
	.wx-footer-btn:focus-visible {
		border-color: var(--color-accent);
	}

	.wx-footer-btn-accent {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.wx-footer-acked {
		display: flex;
		align-items: center;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		padding: 0 var(--space-sm);
	}

	.wx-icon-btn {
		padding: 0 var(--space-sm);
	}

	.wx-footer-menu-wrap {
		position: relative;
		margin-left: auto;
	}

	.wx-footer-menu {
		position: absolute;
		bottom: 100%;
		right: 0;
		z-index: var(--z-overlay);
		min-width: 160px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		padding: var(--space-xs);
		display: flex;
		flex-direction: column;
	}

	.wx-row-menu-item {
		min-height: 40px;
		padding: 0 var(--space-sm);
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
	}

	.wx-row-menu-item:hover,
	.wx-row-menu-item:focus-visible {
		background: var(--color-primary);
	}

	@media (max-width: 480px) {
		.wx-detail-footer {
			display: grid;
			grid-template-columns: 1fr 1fr auto;
			gap: var(--space-xs);
		}

		.wx-footer-btn-accent {
			grid-column: 1 / -1;
			order: -1;
		}

		.wx-footer-acked {
			grid-column: 1 / -1;
			justify-content: center;
			order: -1;
		}

		.wx-footer-menu-wrap {
			margin-left: 0;
		}
	}
</style>
