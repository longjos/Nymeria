<script lang="ts">
	// Read-only NWS alert banner for the agency dashboard (observers only —
	// no ack controls here; NCS acknowledges from the operator app). BUILD-PLAN
	// §5 "Work packages" WP4 / spec-frontend.md §8.8.
	import type { WxAlert } from '$lib/types';
	import { tierMeta } from '$lib/wxAlertMeta';
	import { clock } from '$lib/wxAlertTime';
	import WxTierGlyph from '../WxTierGlyph.svelte';

	let { alert, presentationMode = false }: { alert: WxAlert; presentationMode?: boolean } = $props();

	let meta = $derived(tierMeta[alert.tier]);
</script>

<div class="wx-dashboard-banner wx-tier-{alert.tier}" class:presentation={presentationMode} role="status">
	<div class="wx-band" aria-hidden="true"></div>
	<div class="wx-body">
		<WxTierGlyph tier={alert.tier} size={presentationMode ? 24 : 18} title={alert.event} />
		<span class="wx-event">{alert.event}</span>
		<span class="wx-sep">&middot;</span>
		<span class="wx-affects">affects {alert.affects.summary || 'the watch area'}</span>
		<span class="wx-sep">&middot;</span>
		<span class="wx-ends">ends {clock(alert.endsAt)}</span>
		<span class="wx-sep">&middot;</span>
		<span class="wx-sender">{alert.senderName}</span>
		<span class="wx-sep">&middot;</span>
		<span class="wx-fetched">fetched {clock(alert.fetchedAt)}</span>
	</div>
</div>

<style>
	.wx-dashboard-banner {
		grid-row: 1;
		display: flex;
		align-items: stretch;
		background: var(--color-surface);
		border-bottom: 1px solid var(--color-wx-warning);
	}

	.wx-band {
		width: 6px;
		flex-shrink: 0;
		background: var(--color-wx-warning);
	}
	.wx-tier-watch .wx-band { background: var(--color-wx-watch); }
	.wx-tier-advisory .wx-band { background: var(--color-wx-advisory); }
	.wx-tier-statement .wx-band { background: var(--color-wx-statement); }

	.wx-body {
		flex: 1;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		padding: 10px var(--space-md);
		font-size: 0.9rem;
		color: var(--color-text);
	}

	.presentation .wx-body {
		font-size: 1.25rem;
		padding: 16px var(--space-lg);
	}

	.wx-event {
		font-weight: 800;
		text-transform: uppercase;
		color: var(--color-wx-warning);
	}
	.wx-tier-watch .wx-event { color: var(--color-wx-watch); }
	.wx-tier-advisory .wx-event { color: var(--color-wx-advisory); }
	.wx-tier-statement .wx-event { color: var(--color-wx-statement); }

	.wx-sep {
		color: var(--color-text-muted);
	}

	.wx-affects, .wx-ends, .wx-sender, .wx-fetched {
		color: var(--color-text-muted);
		white-space: nowrap;
	}

	.wx-ends {
		color: var(--color-text);
		font-weight: 600;
	}
</style>
