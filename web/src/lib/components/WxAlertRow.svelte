<script lang="ts">
	import type { WxAlert } from '$lib/types';
	import { wxMinute, muteAlert, unmuteAlert, showAlertOnMap } from '$lib/stores/wxAlerts';
	import { showToast } from '$lib/stores/toast';
	import { tierMeta, severityStyle, compass, toastLine, compressAreaDesc } from '$lib/wxAlertMeta';
	import { countdown, countdownTone, clock } from '$lib/wxAlertTime';
	import WxTierGlyph from './WxTierGlyph.svelte';

	let {
		alert,
		acked,
		muted,
		expired = false,
		compact = false,
		onOpen,
		onAck
	}: {
		alert: WxAlert;
		acked: boolean;
		muted: boolean;
		/** Expired/cancelled row — struck-through countdown, dimmed. */
		expired?: boolean;
		/** SituationBoard variant: no office/updated line, Ack button on the right instead of the ⋯ menu. */
		compact?: boolean;
		onOpen: (id: string) => void;
		onAck?: (id: string) => void;
	} = $props();

	let menuOpen = $state(false);
	let menuWrapEl: HTMLElement | null = $state(null);

	// Once-a-minute is enough precision for a list row (the detail view ticks every second).
	let now = $derived($wxMinute * 60000);
	let cd = $derived(countdown(alert.endsAt, now));
	let tone = $derived(countdownTone(alert.endsAt, now));
	let sev = $derived(severityStyle(alert.tier, alert.severity));
	let meta = $derived(tierMeta[alert.tier]);

	// The distinguishing fact between simultaneous same-event alerts (P1-1):
	// area, not WFO/updated-time, which are detail-view facts now. Near
	// alerts keep their "N mi NE of course" prefix — that IS the where-answer
	// for a nearby (not in-area) alert.
	let whereLine = $derived.by(() => {
		const area = compressAreaDesc(alert.areaDesc);
		const nearPrefix =
			alert.proximity === 'near' ? `${Math.round(alert.distanceMiles)} mi ${compass(alert.bearingDeg)} of course` : '';
		let s = [nearPrefix, area].filter(Boolean).join(' · ') || alert.affects.summary || 'in watch area';
		if (alert.ackedForNet) s += ` · acked for net by ${alert.ackedForNet.callsign || alert.ackedForNet.userName}`;
		if (alert.endedReason === 'clock') s += ' · expired by clock — no NWS confirmation';
		return s;
	});

	let ariaLabel = $derived(
		`${alert.event}, ${alert.tier}, ${acked ? 'acknowledged' : 'unacknowledged'}, ends ${clock(alert.endsAt)}, ${whereLine}`
	);

	function toggleMenu(e: MouseEvent): void {
		e.stopPropagation();
		menuOpen = !menuOpen;
	}

	function closeMenu(): void {
		menuOpen = false;
	}

	function handleAck(e: MouseEvent): void {
		e.stopPropagation();
		onAck?.(alert.id);
		closeMenu();
	}

	function handleMuteToggle(e: MouseEvent): void {
		e.stopPropagation();
		if (muted) unmuteAlert(alert.id);
		else muteAlert(alert.id);
		closeMenu();
	}

	function handleShowOnMap(e: MouseEvent): void {
		e.stopPropagation();
		showAlertOnMap(alert.id);
		closeMenu();
	}

	async function handleCopySummary(e: MouseEvent): Promise<void> {
		e.stopPropagation();
		closeMenu();
		try {
			await navigator.clipboard.writeText(toastLine(alert));
			showToast('Copied.', 'success', 2000);
		} catch {
			// Clipboard API may be unavailable (permissions, non-secure context) — fail silently.
		}
	}

	$effect(() => {
		if (!menuOpen) return;
		const onDown = (e: PointerEvent) => {
			if (menuWrapEl?.contains(e.target as Node)) return;
			closeMenu();
		};
		window.addEventListener('pointerdown', onDown, true);
		return () => window.removeEventListener('pointerdown', onDown, true);
	});
</script>

<!--
	A non-interactive `role="listitem"` wrapper around two sibling <button>s
	(the row action, the ⋯ menu) rather than a `role="button"` wrapper with a
	nested <button> — ARIA forbids interactive descendants inside role=button,
	and a real <button> inside a <button> is a build error in this codebase
	anyway. `[data-wx-row]` stays on the focusable element (.wx-row-main) so
	WxAlertPanel's arrow-key/Home/End/"a" list navigation (querySelector +
	.focus()) keeps working unmodified.
-->
<div
	class="wx-row"
	class:wx-row-expired={expired}
	data-wx-alert-id={alert.id}
	role="listitem"
	style="border-left-color: var(--color-wx-{alert.tier})"
>
	{#if !acked}<span class="wx-new-dot" title="Unacknowledged" aria-label="Unacknowledged"></span>{/if}
	<button
		type="button"
		class="wx-row-main"
		data-wx-row
		data-wx-alert-id={alert.id}
		onclick={() => onOpen(alert.id)}
		aria-label={ariaLabel}
	>
		<span class="wx-row-glyph" class:wx-row-glyph-hatched={sev.hatch} style="color: var({meta.colorVar})">
			<WxTierGlyph tier={alert.tier} size={16} />
		</span>
		<span class="wx-row-body">
			<span class="wx-row-line1">
				<span class="wx-row-event" style="font-weight: {sev.fontWeight}; text-transform: {sev.uppercase ? 'uppercase' : 'none'}">
					{alert.event}
				</span>
				{#if alert.geometrySource === 'zone'}<span class="wx-tag">ZONE</span>{/if}
				{#if alert.status !== 'Actual'}<span class="wx-tag wx-tag-test">TEST</span>{/if}
			</span>
			{#if !compact}
				<span class="wx-row-where">{whereLine}</span>
			{/if}
		</span>
		<span
			class="wx-row-time"
			class:soon={tone === 'soon' && !expired}
			class:wx-row-expired-text={expired || tone === 'expired'}
		>
			<span class="wx-row-countdown">{expired ? 'expired' : cd.text}</span>
			{#if !compact && !expired}<span class="wx-row-ends">ends {clock(alert.endsAt)}</span>{/if}
		</span>
	</button>

	{#if compact}
		{#if onAck && !acked}
			<button type="button" class="wx-row-ack" onclick={handleAck}>Ack</button>
		{/if}
	{:else}
		<span class="wx-row-menu-wrap" bind:this={menuWrapEl}>
			<button
				type="button"
				class="wx-row-menu-btn"
				aria-haspopup="menu"
				aria-expanded={menuOpen}
				aria-label="More actions"
				onclick={toggleMenu}
			>
				⋯
			</button>
			{#if menuOpen}
				<div class="wx-row-menu" role="menu" data-blocks-escape="true">
					{#if !acked}
						<button type="button" role="menuitem" class="wx-row-menu-item" onclick={handleAck}>Acknowledge</button>
					{/if}
					{#if alert.notifyClass !== 'interrupt'}
						<button type="button" role="menuitem" class="wx-row-menu-item" onclick={handleMuteToggle}>
							{muted ? 'Unmute' : 'Mute this alert'}
						</button>
					{/if}
					<button type="button" role="menuitem" class="wx-row-menu-item" onclick={handleShowOnMap}>Show on map</button>
					<button type="button" role="menuitem" class="wx-row-menu-item" onclick={handleCopySummary}>Copy summary</button>
				</div>
			{/if}
		</span>
	{/if}
</div>

<style>
	.wx-row {
		display: flex;
		align-items: stretch;
		width: 100%;
		min-height: 44px;
		padding-right: var(--space-xs);
		background: var(--color-surface);
		border: none;
		border-left: 3px solid;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		position: relative;
	}

	.wx-row-expired {
		opacity: 0.5;
	}

	/* Unacknowledged marker lives in the gutter next to the tier border, as a
	   row state — not a badge competing with the countdown for attention. */
	.wx-new-dot {
		position: absolute;
		left: 9px;
		top: 50%;
		transform: translateY(-50%);
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--color-accent);
		pointer-events: none;
	}

	.wx-row-main {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: flex-start;
		gap: var(--space-sm);
		min-height: 44px;
		padding: var(--space-xs) var(--space-sm) var(--space-xs) 14px;
		background: none;
		border: none;
		color: inherit;
		text-align: left;
		font: inherit;
		cursor: pointer;
	}

	.wx-row-main:hover,
	.wx-row-main:focus-visible {
		background: var(--color-primary);
	}

	.wx-row-glyph {
		flex-shrink: 0;
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-sm);
	}

	.wx-row-glyph-hatched {
		background: repeating-linear-gradient(45deg, currentColor 0, currentColor 1px, transparent 1px, transparent 5px);
		opacity: 0.9;
	}

	.wx-row-body {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.wx-row-line1 {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.wx-row-event {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.85rem;
	}

	.wx-tag {
		flex-shrink: 0;
		font-size: 0.6rem;
		font-weight: 700;
		border: 1px solid var(--color-text-muted);
		color: var(--color-text-muted);
		border-radius: 8px;
		padding: 1px 6px;
	}

	.wx-tag-test {
		border-color: var(--color-wx-watch);
		color: var(--color-wx-watch);
	}

	.wx-row-where {
		font-size: 0.75rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Countdown promoted to the row's second-strongest element: bold, the
	   text color (not muted), and the size that rhymes with the detail
	   view's timing hero — the two views should visually agree. */
	.wx-row-time {
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 1px;
		padding-top: 1px;
	}

	.wx-row-countdown {
		font-size: 0.9rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		color: var(--color-text);
		line-height: 1.1;
	}

	.wx-row-ends {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.wx-row-time.soon .wx-row-countdown {
		color: var(--color-wx-watch);
	}

	.wx-row-time.wx-row-expired-text .wx-row-countdown {
		color: var(--color-wx-expired);
		text-decoration: line-through;
	}

	.wx-row-ack {
		flex-shrink: 0;
		align-self: center;
		min-height: 32px;
		padding: 0 var(--space-sm);
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-sm);
		color: white;
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-row-menu-wrap {
		position: relative;
		flex-shrink: 0;
		display: flex;
		align-items: center;
	}

	/* 44px touch target (P1-5) — 32px was not enough for a phone in a moving vehicle. */
	.wx-row-menu-btn {
		width: 44px;
		height: 44px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 1rem;
		cursor: pointer;
	}

	.wx-row-menu-btn:hover,
	.wx-row-menu-btn:focus-visible {
		background: var(--color-bg);
		color: var(--color-text);
	}

	.wx-row-menu {
		position: absolute;
		top: 100%;
		right: 0;
		z-index: var(--z-overlay);
		min-width: 180px;
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
</style>
