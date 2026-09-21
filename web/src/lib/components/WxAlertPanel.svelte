<script lang="ts">
	import type { WxAlert } from '$lib/types';
	import {
		wxInAreaAlerts,
		wxNearbyAlerts,
		wxExpiredAlerts,
		wxIsAcked,
		wxMuted,
		wxFilter,
		wxUnackedOnly,
		wxSelectedAlertId,
		wxLinkStatus,
		wxFootprint,
		ackAlert
	} from '$lib/stores/wxAlerts';
	import { sheetState, openSettings } from '$lib/stores/ui';
	import { userRole } from '$lib/stores/session';
	import { clock } from '$lib/wxAlertTime';
	import WxProvenanceStrip from './WxProvenanceStrip.svelte';
	import WxAlertRow from './WxAlertRow.svelte';
	import WxAlertDetail from './WxAlertDetail.svelte';
	import WxTierGlyph from './WxTierGlyph.svelte';

	let { onFlyTo }: { onFlyTo?: (lat: number, lon: number) => void } = $props();

	// Statements are shown under "All" only — not a selectable filter chip.
	const FILTERS: Array<{ key: 'all' | 'warning' | 'watch' | 'advisory'; label: string }> = [
		{ key: 'all', label: 'All' },
		{ key: 'warning', label: 'Warnings' },
		{ key: 'watch', label: 'Watches' },
		{ key: 'advisory', label: 'Advisories' }
	];

	function matchesFilter(a: WxAlert): boolean {
		return $wxFilter === 'all' || a.tier === $wxFilter;
	}

	let inArea = $derived($wxInAreaAlerts.filter((a) => matchesFilter(a) && (!$wxUnackedOnly || !$wxIsAcked(a))));
	let nearby = $derived($wxNearbyAlerts.filter((a) => matchesFilter(a) && (!$wxUnackedOnly || !$wxIsAcked(a))));
	let expiredList = $derived($wxExpiredAlerts.filter((a) => matchesFilter(a)));
	let nearbyOpen = $derived(inArea.length < 3);

	let selected = $derived.by(() => {
		const id = $wxSelectedAlertId;
		if (!id) return null;
		return [...$wxInAreaAlerts, ...$wxNearbyAlerts, ...$wxExpiredAlerts].find((a) => a.id === id) ?? null;
	});

	let zoneNamesText = $derived.by(() => {
		const zones = $wxFootprint?.zones ?? [];
		return zones.length ? zones.map((z) => z.name || z.ugc).join(', ') : 'watch area not resolved yet';
	});

	function openDetail(id: string): void {
		lastOpenedId = id;
		wxSelectedAlertId.set(id);
		sheetState.set('full');
	}

	function closeDetail(): void {
		wxSelectedAlertId.set(null);
		// P0-3: the detail unmounts and the list remounts, so without this the
		// keyboard user who arrowed to row 7 lands back at <body>.
		wantRowFocus = true;
		// 'half' puts the list below the fold on a phone — the chrome above it
		// is taller than the space left over — so coming back from a detail
		// landed on no visible rows at all.
		sheetState.set('full');
	}

	function handleAck(id: string): void {
		void ackAlert(id);
	}

	let listEl: HTMLElement | undefined = $state();
	let lastOpenedId = $state<string | null>(null);
	let wantRowFocus = $state(false);

	$effect(() => {
		if (!wantRowFocus || !listEl || selected) return;
		const row = lastOpenedId
			? listEl.querySelector<HTMLElement>(`[data-wx-row][data-wx-alert-id="${CSS.escape(lastOpenedId)}"]`)
			: null;
		(row ?? listEl.querySelector<HTMLElement>('[data-wx-row]'))?.focus();
		wantRowFocus = false;
	});

	function handleListKeydown(e: KeyboardEvent): void {
		if (!listEl) return;
		const rows = Array.from(listEl.querySelectorAll<HTMLElement>('[data-wx-row]'));
		if (rows.length === 0) return;
		const idx = rows.indexOf(document.activeElement as HTMLElement);

		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				rows[Math.min(rows.length - 1, idx + 1)]?.focus();
				break;
			case 'ArrowUp':
				e.preventDefault();
				rows[Math.max(0, idx - 1)]?.focus();
				break;
			case 'Home':
				e.preventDefault();
				rows[0]?.focus();
				break;
			case 'End':
				e.preventDefault();
				rows[rows.length - 1]?.focus();
				break;
			case 'a': {
				if (idx < 0) break;
				const id = rows[idx].getAttribute('data-wx-alert-id');
				if (id) handleAck(id);
				break;
			}
			default:
				break;
		}
	}
</script>

{#if selected}
	<WxAlertDetail alert={selected} onBack={closeDetail} {onFlyTo} />
{:else}
	<div class="wx-alert-panel">
		<WxProvenanceStrip />

		{#if $wxLinkStatus.state === 'off'}
			<!-- Off is a state, not a failure. The strip above already names the
			     status and the footer below covers the RF caveat, so this says
			     only what neither does: the consequence, and the way out. -->
			<div class="wx-off-state">
				<h3 class="wx-off-title">Not monitoring NWS alerts</h3>
				<p class="wx-off-body">
					{#if $wxLinkStatus.reason === 'noWatchArea'}
						No watch area has resolved yet, so nothing is being fetched. Open a net, or
						set home zones in Settings.
					{:else if $wxLinkStatus.reason === 'initFailed'}
						Alerts are on, but the service didn't start. Check the server log.
					{:else if $wxLinkStatus.reason === 'contactMissing'}
						The NWS needs a contact address before it will accept requests.
					{:else}
						No watches or warnings will appear here. This is not an all-clear.
					{/if}
				</p>
				{#if $userRole === 'admin'}
					<button class="wx-off-action" onclick={() => openSettings('wxAlerts')}>
						{$wxLinkStatus.reason === 'contactMissing' ? 'Add contact in Settings' : 'Open Settings'}
					</button>
				{:else}
					<p class="wx-off-hint">An admin can change this in Settings.</p>
				{/if}
			</div>
		{:else}
		<div class="wx-filter-bar" role="group" aria-label="Filter alerts">
			{#each FILTERS as f (f.key)}
				<button class="wx-filter-chip" class:active={$wxFilter === f.key} onclick={() => wxFilter.set(f.key)}>{f.label}</button>
			{/each}
			<button
				class="wx-filter-chip"
				aria-pressed={$wxUnackedOnly}
				class:active={$wxUnackedOnly}
				onclick={() => wxUnackedOnly.update((v) => !v)}
			>
				Unacknowledged
			</button>
		</div>

		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<!-- `role="list"` sits on the direct parent of the rows, one per group:
		     on the groups container the listitems were grandchildren, so no
		     screen reader counted them. The container is the group that owns
		     arrow-key/Home/End navigation across all three lists. -->
		<div
			class="wx-alert-groups"
			role="group"
			aria-label="NWS alerts"
			bind:this={listEl}
			onkeydown={handleListKeydown}
		>
			<section class="wx-group">
				<h3 class="wx-group-heading" id="wx-group-inarea">In watch area <span class="wx-group-count">{inArea.length}</span></h3>
				{#if inArea.length === 0}
					<p class="wx-group-empty" class:wx-group-empty-down={$wxLinkStatus.state === 'down'}>
						{#if $wxLinkStatus.state === 'down'}
							No alerts known · NWS link down since {clock($wxLinkStatus.lastSuccessAt)} — this is NOT an all-clear
						{:else}
							No active alerts in the watch area · {zoneNamesText}
						{/if}
					</p>
				{:else}
					<div role="list" aria-labelledby="wx-group-inarea">
						{#each inArea as a (a.id)}
							<WxAlertRow alert={a} acked={$wxIsAcked(a)} muted={$wxMuted.has(a.id)} onOpen={openDetail} onAck={handleAck} />
						{/each}
					</div>
				{/if}
			</section>

			<details class="wx-group" open={nearbyOpen}>
				<summary class="wx-group-heading" id="wx-group-nearby">Nearby <span class="wx-group-count">{nearby.length}</span></summary>
				<div role="list" aria-labelledby="wx-group-nearby">
					{#each nearby as a (a.id)}
						<WxAlertRow alert={a} acked={$wxIsAcked(a)} muted={$wxMuted.has(a.id)} onOpen={openDetail} onAck={handleAck} />
					{/each}
				</div>
			</details>

			<details class="wx-group">
				<summary class="wx-group-heading" id="wx-group-expired">Expired · last hour <span class="wx-group-count">{expiredList.length}</span></summary>
				<div role="list" aria-labelledby="wx-group-expired">
					{#each expiredList as a (a.id)}
						<WxAlertRow alert={a} acked={true} muted={$wxMuted.has(a.id)} expired onOpen={openDetail} />
					{/each}
				</div>
			</details>
		</div>

		<div class="wx-legend">
			<span class="wx-legend-item"><WxTierGlyph tier="warning" size={12} /> Warning</span>
			<span class="wx-legend-item"><WxTierGlyph tier="watch" size={12} /> Watch</span>
			<span class="wx-legend-item"><WxTierGlyph tier="advisory" size={12} /> Advisory</span>
			<span class="wx-legend-item"><span class="wx-legend-hatch" aria-hidden="true"></span> hatched = severe/extreme</span>
		</div>
			<p class="wx-source-footer">Source: NWS API over the internet. Stations without internet receive alerts only when NCS relays them.</p>
		{/if}
	</div>
{/if}

<style>
	.wx-alert-panel {
		display: flex;
		flex-direction: column;
	}

	.wx-filter-bar {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
	}

	.wx-filter-chip {
		min-height: 32px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		color: var(--color-text-muted);
		font-size: 0.75rem;
		cursor: pointer;
	}

	/* Mobile review P1-2: 32px chips are under Apple's 44px minimum for a
	   field operator tapping one-handed. Gated to the phone breakpoint (the
	   same one that switches BottomSheet/SidePanel — see app.css
	   .mobile-only/.desktop-only) rather than `pointer: coarse`, so a
	   touchscreen desktop/laptop viewport never picks this up and desktop
	   density is untouched. */
	@media (max-width: 768px) {
		/* One scrollable row, not two wrapped rows. At 44px-per-chip the wrap
		   cost ~200px of a 852px screen — enough that the sheet at `half`
		   showed a single alert. Same horizontal-scroll pattern the mobile
		   toolbar already uses. */
		.wx-filter-bar {
			flex-wrap: nowrap;
			overflow-x: auto;
			overscroll-behavior-x: contain;
			scroll-snap-type: x proximity;
			scrollbar-width: none;
			-webkit-overflow-scrolling: touch;
		}
		.wx-filter-bar::-webkit-scrollbar {
			display: none;
		}
		.wx-filter-chip {
			flex: 0 0 auto;
			scroll-snap-align: start;
			min-height: 44px;
			padding: 0 var(--space-md);
		}
	}

	.wx-filter-chip.active {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}

	.wx-off-state {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
	}
	.wx-off-title {
		margin: 0;
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.wx-off-body {
		margin: 0.5rem auto 0;
		max-width: 30ch;
		font-size: 0.8rem;
		line-height: 1.5;
	}
	.wx-off-action {
		margin-top: var(--space-md);
		padding: 6px 12px;
		min-height: 32px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.75rem;
		cursor: pointer;
	}
	.wx-off-action:hover {
		background: var(--color-surface);
	}
	.wx-off-hint {
		margin: var(--space-md) 0 0;
		font-size: 0.75rem;
		opacity: 0.7;
	}

	.wx-alert-groups {
		display: flex;
		flex-direction: column;
		gap: var(--space-lg);
		padding: var(--space-sm) var(--space-md) var(--space-md);
	}

	.wx-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	/* The rows themselves. Without this the list wrapper had no gap at all and
	   every alert butted against the next one — a wall of colour bars with no
	   edge to read against. */
	.wx-group [role='list'] {
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.wx-group-heading {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		color: var(--color-text-muted);
		cursor: default;
	}

	summary.wx-group-heading {
		cursor: pointer;
	}

	/* Mobile review P1-2: the <summary> is the ONLY way to open "Nearby" /
	   "Expired · last hour" and measured 13px tall — worst tap target in the
	   panel. min-height on the already-flex heading grows the whole row
	   (summary is a block-level flex item stretched to the group's width, so
	   the full width becomes tappable, not just the glyph). The plain <h3>
	   heading picks up the same rule so the two heading styles keep the same
	   height. Phone-only, same rationale as .wx-filter-chip above. */
	@media (max-width: 768px) {
		.wx-group-heading {
			min-height: 44px;
			padding: 0 var(--space-xs);
			border-radius: var(--radius-sm);
		}
	}

	.wx-group-count {
		background: var(--color-primary);
		border-radius: var(--radius-full);
		padding: 1px 7px;
		font-size: 0.65rem;
	}

	.wx-group-empty {
		font-size: 0.8rem;
		color: var(--color-text-muted);
		padding: var(--space-sm);
	}

	.wx-group-empty-down {
		color: var(--color-wx-watch);
	}

	.wx-legend {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-md);
		padding: var(--space-sm) var(--space-md);
		border-top: 1px solid var(--color-primary);
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-legend-item {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}

	.wx-legend-hatch {
		width: 12px;
		height: 12px;
		display: inline-block;
		background-image: repeating-linear-gradient(45deg, currentColor 0, currentColor 1px, transparent 1px, transparent 3px);
	}

	.wx-source-footer {
		padding: 0 var(--space-md) var(--space-md);
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}
</style>
