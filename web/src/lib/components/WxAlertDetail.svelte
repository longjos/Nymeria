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
		showAlertOnMap,
		normalizeAlert
	} from '$lib/stores/wxAlerts';
	import { showToast } from '$lib/stores/toast';
	import { tierMeta, zoneLabel, toastLine, compressAreaDesc, proximityLabel, proximityNote } from '$lib/wxAlertMeta';
	import { clock, clockWithDay, clockWithSeconds, countdown, countdownTone, progress } from '$lib/wxAlertTime';
	import WxTierGlyph from './WxTierGlyph.svelte';
	import WxRelayComposer from './WxRelayComposer.svelte';
	import { nwsBlocks, previewBlocks, type WxBlock } from '$lib/wxAlertText';

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
	// P0-3: every action used to replace the focused control with a <span>,
	// dropping focus to <body>. These are focused explicitly instead.
	let titleEl = $state<HTMLElement | null>(null);
	let ackStatusEl = $state<HTMLElement | null>(null);
	let relayBtnEl = $state<HTMLElement | null>(null);
	let wantAckFocus = $state(false);

	let displayed = $derived(showingPrevious ?? alert);

	// P1-5: 'issued 3:09 PM · effective 3:09 PM' is the same value twice on the
	// vast majority of NWS products. Only show effective when it differs.
	let effectiveDiffers = $derived(clock(displayed.effective) !== clock(displayed.sent));
	// P1-6: a Tornado Emergency is the most important upgrade NWS issues, and
	// the detail title was dropping it.
	let title = $derived(displayed.effectiveEvent || displayed.event);
	let titleSub = $derived(displayed.effectiveEvent && displayed.effectiveEvent !== displayed.event ? displayed.event : '');

	// Built as one string: Svelte trims the leading space off a block's first
	// text node, which silently glued "3:09 PM" to "· updated".
	let timingSub = $derived.by(() => {
		const parts = [`issued ${clockWithDay(displayed.sent, clockNow)}`];
		if (effectiveDiffers) parts.push(`effective ${clockWithDay(displayed.effective, clockNow)}`);
		if (!showingPrevious && displayed.references.length > 0) parts.push(`updated ${clock(displayed.updatedAt)}`);
		return parts.join(' · ');
	});



	// NWS text normalisation (mobile review P1-1): the raw instruction/
	// description are hard-wrapped at ~68 columns. nwsBlocks() joins only
	// those wrap artifacts and keeps the structural line breaks (bullets,
	// sub-bullets, label lines, terminators, gauge tables) intact.
	let instructionBlocks = $derived(displayed.instruction ? nwsBlocks(displayed.instruction) : []);
	let headlineBlocks = $derived(displayed.headline ? nwsBlocks(displayed.headline) : []);
	let descriptionBlocks = $derived(nwsBlocks(displayed.description));
	let descriptionPreview = $derived(previewBlocks(descriptionBlocks));
	let descriptionHasMore = $derived(descriptionPreview.length < descriptionBlocks.length);
	let descriptionShown = $derived(showMoreDescription || !descriptionHasMore ? descriptionBlocks : descriptionPreview);

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

	// P0-3 / P2-7: opening a row unmounts the list, so focus has nowhere to go
	// unless the detail claims it. tabindex="-1" keeps it out of the tab order.
	$effect(() => {
		titleEl?.focus();
	});

	// Move focus onto the acknowledged status the moment it replaces the button
	// it was on; as an <output> it announces itself to a screen reader too.
	$effect(() => {
		if (wantAckFocus && ackStatusEl) {
			ackStatusEl.focus();
			wantAckFocus = false;
		}
	});

	async function loadHistory(): Promise<void> {
		if (history) {
			showingPrevious = history[0] ?? null;
			return;
		}
		historyLoading = true;
		try {
			const res = await api.wxAlert(alert.id);
			// History bypasses applyAlerts, so normalise it here too.
			history = (res.history ?? []).map(normalizeAlert);
			showingPrevious = history[0] ?? null;
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
		menuOpen = false;
		wantAckFocus = true;
		await ackAlert(alert.id);
	}

	async function handleAckForNet(): Promise<void> {
		try {
			wantAckFocus = true;
			await ackAlertForNet(alert.id);
			showToast('Acknowledged for the net — the banner is cleared for every station.', 'success');
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
			showToast('Clipboard blocked — select and copy the text instead.', 'error');
		}
	}

	function openOnWeatherGov(): void {
		menuOpen = false;
		// providerUrl is the feature URL NWS itself published for this alert
		// (types.ts documents it for exactly this link); the hand-built search
		// URL was a second source of truth for the same thing.
		const url = alert.providerUrl || `https://alerts.weather.gov/search?id=${encodeURIComponent(alert.id)}`;
		window.open(url, '_blank', 'noopener');
	}

	function closeRelay(): void {
		relayOpen = false;
		relayBtnEl?.focus();
	}
</script>

{#snippet nwsBlockList(blocks: WxBlock[])}
	{#each blocks as block}
		{#if block.kind === 'pre'}
			<pre class="wx-table">{block.text}</pre>
		{:else if block.kind === 'item'}
			<p class="wx-item" class:sub={block.sub}>
				{#if block.label}<span class="wx-item-label">{block.label}</span>{/if}{block.text}
			</p>
		{:else}
			<p class="wx-p">{block.text}</p>
		{/if}
	{/each}
{/snippet}

<div class="wx-detail">
	{#if showingPrevious}
		<div class="wx-detail-breadcrumb">
			<button class="wx-breadcrumb-link" onclick={onBack}>Alerts</button>
			›
			<button class="wx-breadcrumb-link" onclick={backToCurrent}>{alert.effectiveEvent || alert.event}</button>
			› earlier version
		</div>
	{/if}

	<div class="wx-detail-scroll">
		{#if showingPrevious}
			<div class="wx-detail-band wx-band-previous">EARLIER VERSION · sent {clock(showingPrevious.sent)}</div>
		{:else}
			<div class="wx-detail-band" style="background: var({meta.colorVar})" class:wx-band-hatched={displayed.severity === 'Extreme' || displayed.severity === 'Severe'}></div>
		{/if}

		<div class="wx-detail-header">
			<button class="wx-back" onclick={onBack} aria-label="Back to alert list">
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
					<path d="M10 3L5 8l5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
				</svg>
				<span>Alerts</span>
			</button>
			<WxTierGlyph tier={displayed.tier} size={20} title={meta.ariaWord} />
			<div class="wx-detail-titles">
				<h2 class="wx-detail-title" bind:this={titleEl} tabindex="-1">{title}</h2>
				{#if titleSub}<p class="wx-detail-title-sub">{titleSub} · upgraded</p>{/if}
			</div>
		</div>

		<div class="wx-detail-chips">
			<span class="wx-chip wx-chip-proximity">{proximityLabel(displayed)}</span>
			<span class="wx-chip"><span class="wx-chip-key">Severity</span> {displayed.severity}</span>
			<span class="wx-chip"><span class="wx-chip-key">Certainty</span> {displayed.certainty}</span>
			<span class="wx-chip"><span class="wx-chip-key">Urgency</span> {displayed.urgency}</span>
			{#if displayed.geometrySource === 'zone'}<span class="wx-chip wx-chip-zone">ZONE</span>{/if}
			{#if displayed.status !== 'Actual'}<span class="wx-chip wx-chip-test">TEST</span>{/if}
		</div>

		<div class="wx-timing" role="group" aria-label={isEnded ? `Ended ${clockWithDay(displayed.endedAt ?? displayed.endsAt, clockNow)}` : `Ends ${clockWithDay(displayed.endsAt, clockNow)}`}>
			{#if isEnded}
				<div class="wx-timing-row">
					<span class="wx-timing-label">ENDED {clockWithDay(displayed.endedAt ?? displayed.endsAt, clockNow)}</span>
				</div>
			{:else}
				<div class="wx-timing-row">
					<span class="wx-timing-label">ENDS {clockWithDay(displayed.endsAt, clockNow)}</span>
					<time datetime={displayed.endsAt} class="wx-timing-countdown" class:soon={tone === 'soon'} class:urgent={tone === 'urgent'} aria-live="off">{cd.text}</time>
				</div>
			{/if}
			<div class="wx-timing-bar" class:saturate={isEnded}>
				<div class="wx-timing-fill" style="width: {bar * 100}%; background: var({meta.colorVar})"></div>
			</div>
			<div class="wx-timing-sub">{timingSub}</div>
			{#if !showingPrevious && displayed.references.length > 0}
				<button class="wx-replaces" onclick={loadHistory} disabled={historyLoading}>
					↺ {historyLoading ? 'Loading…' : `Replaces earlier alert (${clock(displayed.references[0].sent)})`}
				</button>
			{/if}
		</div>

		{#if displayed.instruction}
			<div class="wx-instruction" style="background: var({meta.softVar}); border-left-color: var({meta.colorVar})">
				<h3 class="wx-section-title" style="color: var({meta.textVar})">Instruction</h3>
				{@render nwsBlockList(instructionBlocks)}
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
				<p class="wx-empty-note">{proximityNote(displayed)}</p>
			</div>
		{/if}

		<div class="wx-section">
			<h3 class="wx-section-title">Description</h3>
			{@render nwsBlockList(descriptionShown)}
			{#if descriptionHasMore}
				<button class="wx-show-more" aria-expanded={showMoreDescription} onclick={() => (showMoreDescription = !showMoreDescription)}>
					{showMoreDescription ? 'Show less' : 'Show more'}
				</button>
			{/if}
		</div>

		<div class="wx-section">
			<h3 class="wx-section-title">Affected area (NWS)</h3>
			<p>{compressAreaDesc(displayed.areaDesc)}</p>
			{#if displayed.ugc.length}<p class="wx-ugc">{displayed.ugc.join(', ')}</p>{/if}
			{#if compressAreaDesc(displayed.areaDesc) !== displayed.areaDesc}
				<p class="wx-areadesc-raw">{displayed.areaDesc}</p>
			{/if}
		</div>

		<div class="wx-section wx-source">
			<p>{displayed.senderName} ({displayed.senderId})</p>
			<p class="wx-source-sub">sent {clock(displayed.sent)} · fetched {clockWithSeconds(displayed.fetchedAt)}</p>
			{#if displayed.headline}
				<p class="wx-source-sub wx-source-headline">{headlineBlocks.map((b) => b.text).join(' ')}</p>
			{/if}
			{#if muted}
				<p class="wx-source-sub">Muted on this device — escalations still come through.</p>
			{/if}
			{#if displayed.endedReason === 'dropped'}
				<p class="wx-source-sub">no longer listed by NWS</p>
			{:else if displayed.endedReason === 'clock'}
				<p class="wx-source-sub">expired by clock — no NWS confirmation</p>
			{/if}
		</div>
	</div>

	<!--
		Fixed slots, always in the same order and always present: status line,
		then [back] [show on map] [primary] [relay] [⋯], then the caption. The
		footer used to add and remove whole controls as the alert's state
		changed — acking swapped a button for a span and everything after it
		slid sideways — so the same action lived in a different place depending
		on what you had already done.
	-->
	<div class="wx-detail-footer" class:wx-footer-previous={!!showingPrevious}>
		{#if showingPrevious}
			<button class="wx-footer-btn wx-footer-btn-primary wx-footer-slot-primary" onclick={backToCurrent}>
				Back to current version
			</button>
		{:else}
			<output class="wx-footer-status" bind:this={ackStatusEl} tabindex="-1">
				{#if alert.ackedForNet}
					<span class="wx-status-dot wx-status-done" aria-hidden="true"></span>
					Acked for net by {alert.ackedForNet.callsign || alert.ackedForNet.userName} · {clock(alert.ackedForNet.at)}
				{:else if acked}
					<span class="wx-status-dot wx-status-done" aria-hidden="true"></span>
					Acknowledged{ackedAt ? ` ${clock(ackedAt)}` : ''}{$wxIsNcs ? ' · not yet acked for net' : ''}
				{:else}
					<span class="wx-status-dot" aria-hidden="true"></span>
					Not acknowledged
				{/if}
			</output>

			<div class="wx-footer-actions">
				<button class="wx-footer-btn wx-footer-back" onclick={onBack} aria-label="Back to alert list">‹ Alerts</button>
				<button class="wx-footer-btn wx-footer-showmap" onclick={() => showAlertOnMap(alert.id)}>Show on map</button>

				<!-- The primary slot never empties; it goes disabled once the
				     strongest available acknowledgement has been made, so the
				     control keeps its position. -->
				{#if $wxIsNcs}
					<button
						class="wx-footer-btn wx-footer-btn-primary wx-footer-slot-primary"
						disabled={!!alert.ackedForNet}
						onclick={handleAckForNet}
					>
						{alert.ackedForNet ? 'Acknowledged for net' : 'Acknowledge for net'}
					</button>
					<button class="wx-footer-btn wx-footer-slot-relay" bind:this={relayBtnEl} onclick={() => (relayOpen = true)}>
						Relay to net…
					</button>
				{:else}
					<button
						class="wx-footer-btn wx-footer-btn-primary wx-footer-slot-primary"
						disabled={acked}
						onclick={handleAcknowledge}
					>
						{acked ? 'Acknowledged' : 'Acknowledge'}
					</button>
				{/if}

				<div class="wx-footer-menu-wrap" bind:this={menuWrapEl}>
					<button class="wx-footer-btn wx-icon-btn" aria-haspopup="true" aria-expanded={menuOpen} aria-label="More actions" onclick={() => (menuOpen = !menuOpen)}>
						⋯
					</button>
					{#if menuOpen}
						<div class="wx-footer-menu" data-blocks-escape="true">
							{#if $wxIsNcs && !acked}
								<button class="wx-row-menu-item" onclick={handleAcknowledge}>Acknowledge on this device only</button>
							{/if}
							{#if alert.notifyClass !== 'interrupt'}
								<button class="wx-row-menu-item" onclick={handleMuteToggle}>{muted ? 'Unmute' : 'Mute'}</button>
							{/if}
							<button class="wx-row-menu-item" onclick={handleCopySummary}>Copy summary</button>
							<button class="wx-row-menu-item" onclick={openOnWeatherGov}>Open on weather.gov</button>
						</div>
					{/if}
				</div>
			</div>

			<p class="wx-footer-caption">
				{#if $wxIsNcs}
					Acknowledging for the net clears the banner for every station and logs it to the net.
				{:else}
					Acknowledging is local to this device.
				{/if}
			</p>
		{/if}
	</div>
</div>

{#if relayOpen}
	<WxRelayComposer {alert} onClose={closeRelay} />
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
		/* Accent-on-surface is ~4.15:1 — below AA at this size. Full-strength
		   text with an accent underline carries the affordance instead. */
		color: var(--color-text);
		text-decoration: underline;
		text-decoration-color: var(--color-accent);
		text-underline-offset: 2px;
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

	/* The hatch has a 6px period, so it needs more than 4px to read as a hatch
	   at all — this is the loudest place the "hatched = severe" legend runs. */
	.wx-band-hatched {
		height: 8px;
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
		color: var(--color-text);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-back:hover,
	.wx-back:focus-visible {
		background: var(--color-primary);
		color: var(--color-text);
	}

	.wx-band-previous {
		height: auto;
		padding: 3px var(--space-md);
		background: var(--color-wx-expired);
		color: var(--color-bg);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.04em;
	}

	.wx-detail-titles {
		flex: 1;
		min-width: 0;
	}

	.wx-detail-title {
		font-size: 1rem;
		font-weight: 700;
	}

	.wx-detail-title:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}

	.wx-detail-title-sub {
		font-size: 0.7rem;
		color: var(--color-text-muted);
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

	.wx-chip-proximity {
		border-color: var(--color-text);
		color: var(--color-text);
	}

	/* "Observed"/"Immediate" are CAP vocabulary and opaque on their own. */
	.wx-chip-key {
		font-weight: 400;
		opacity: 0.75;
		margin-right: 3px;
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

	.wx-timing-countdown.urgent {
		color: var(--color-wx-warning-text);
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

	/* This is the "what to do" text — it has to be readable at arm's length on
	   a phone in sunlight, so it steps up from the description's 0.9rem. */
	.wx-instruction :global(.wx-p),
	.wx-instruction :global(.wx-item) {
		font-size: 0.95rem;
	}

	.wx-instruction :global(.wx-p:first-of-type) {
		font-weight: 600;
	}

	.wx-p,
	.wx-item {
		font-size: 0.9rem;
		line-height: 1.45;
		margin: 0 0 var(--space-sm);
	}

	.wx-item-label {
		display: block;
		font-size: 0.7rem;
		font-weight: 700;
		letter-spacing: 0.03em;
		color: var(--color-text-muted);
	}

	.wx-item.sub {
		padding-left: var(--space-md);
		position: relative;
	}

	.wx-item.sub::before {
		content: '–';
		position: absolute;
		left: 4px;
		color: var(--color-text-muted);
	}

	.wx-table {
		font-family: ui-monospace, monospace;
		font-size: 0.7rem;
		line-height: 1.4;
		overflow-x: auto;
		white-space: pre;
		margin: 0 0 var(--space-sm);
	}

	.wx-show-more {
		margin-top: 4px;
		background: none;
		border: none;
		padding: 0;
		color: var(--color-text);
		text-decoration: underline;
		text-decoration-color: var(--color-accent);
		text-underline-offset: 2px;
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-ugc {
		font-family: monospace;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	/* Kept verbatim under the compressed line for copy/paste fidelity. */
	.wx-areadesc-raw {
		margin-top: 2px;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-source-headline {
		font-style: italic;
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

	/* Three fixed rows: status, actions, caption. Nothing is added or removed
	   as state changes, so no control ever moves. */
	.wx-detail-footer {
		position: sticky;
		bottom: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		padding: var(--space-sm) var(--space-md);
		background: var(--color-bg);
		border-top: 1px solid var(--color-primary);
	}

	.wx-footer-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs);
	}

	.wx-footer-status {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		min-height: 20px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.wx-footer-status:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
		border-radius: var(--radius-sm);
	}

	.wx-status-dot {
		width: 7px;
		height: 7px;
		border-radius: var(--radius-full);
		border: 1px solid var(--color-text-muted);
		flex-shrink: 0;
	}

	.wx-status-done {
		background: var(--color-success);
		border-color: var(--color-success);
	}

	.wx-footer-btn:disabled {
		opacity: 0.55;
		cursor: default;
	}

	.wx-footer-btn-primary:disabled {
		background: var(--color-surface);
		border-color: var(--color-primary);
		color: var(--color-text-muted);
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

	/* The one primary action. An outline in the accent colour is this
	   codebase's *secondary* emphasis, and accent-on-surface text is ~4.15:1 —
	   the fill fixes the hierarchy and the contrast together. */
	.wx-footer-btn-primary {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: var(--color-text);
	}

	.wx-footer-btn-primary:hover,
	.wx-footer-btn-primary:focus-visible {
		filter: brightness(1.1);
	}

	.wx-footer-caption {
		margin: 0;
		min-height: 16px;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-footer-menu-wrap {
		position: relative;
		margin-left: auto;
	}

	/* Mobile-only "back" affordance in the footer's last row (P1-7 — the
	   header .wx-back at the top of the sheet is out of one-handed reach).
	   Hidden on desktop: the header back control already covers it there. */
	.wx-footer-back {
		display: none;
	}

	.wx-icon-btn {
		padding: 0 var(--space-sm);
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
		/* `order` inside .wx-footer-actions, not grid auto-flow: grid
		   auto-placement wrapped the menu trigger into column 1 of its own
		   row, so `right: 0` measured from the wrong box and the menu
		   rendered off the left edge. Each tier's basis sums to ~100% so it
		   always takes its own row: primary (full width) → relay (full
		   width) → back / show-on-map / ⋯ (the reachable last row). The tiers
		   are the same in every state now, because the slots are. */
		.wx-footer-actions {
			display: flex;
			flex-wrap: wrap;
			gap: var(--space-xs);
		}

		.wx-footer-btn {
			order: 2;
			flex: 1 1 calc(50% - var(--space-xs) / 2);
		}

		.wx-footer-btn.wx-footer-slot-primary {
			order: 1;
			flex: 1 1 100%;
			justify-content: center;
		}

		.wx-footer-btn.wx-footer-slot-relay {
			order: 2;
			flex: 1 1 100%;
		}

		.wx-footer-btn.wx-footer-back,
		.wx-footer-btn.wx-footer-showmap {
			display: inline-flex;
			order: 3;
			flex: 1 1 calc((100% - 44px - var(--space-xs) * 2) / 2);
		}

		.wx-footer-menu-wrap {
			order: 3;
			margin-left: 0;
			flex: 0 0 44px;
		}

		/* P1-2: the ⋯ trigger was 31px wide (44px tall only). */
		.wx-icon-btn {
			width: 44px;
			padding: 0;
		}

		/* P1-6: anchor the menu to the wrap's real right edge now that the
		   wrap is reliably the last cell of the last row, with a fallback
		   cap so it can never run past the viewport edge either. */
		.wx-footer-menu {
			right: 0;
			left: auto;
			max-width: calc(100vw - 2 * var(--space-md));
		}

		/* P1-2: sub-44px touch targets inside the detail view. Scoped to
		   mobile widths only so desktop density is unaffected. */
		.wx-show-more {
			display: block;
			width: 100%;
			min-height: 44px;
			text-align: left;
			padding: 0 var(--space-xs);
			margin-top: var(--space-xs);
		}

		.wx-replaces {
			min-height: 44px;
			padding: 0 var(--space-md);
		}

		.wx-chip-btn {
			min-height: 44px;
			padding: 0 var(--space-md);
		}
	}
</style>
