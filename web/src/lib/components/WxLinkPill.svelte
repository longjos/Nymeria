<script lang="ts">
	import { wxLinkStatus, wxClock } from '$lib/stores/wxAlerts';
	import { gpsStatus } from '$lib/stores/gps';
	import { openWeather } from '$lib/stores/ui';
	import { linkStateText } from '$lib/wxAlertTime';

	let visible = $derived($wxLinkStatus.state === 'stale' || $wxLinkStatus.state === 'down');
	let down = $derived($wxLinkStatus.state === 'down');

	function elapsedShort(sinceIso: string | undefined, now: number): string {
		if (!sinceIso) return '';
		const ms = Math.max(0, now - Date.parse(sinceIso));
		const min = Math.round(ms / 60000);
		if (min < 60) return `${min}m`;
		return `${Math.floor(min / 60)}h`;
	}

	let age = $derived(elapsedShort($wxLinkStatus.lastSuccessAt ?? $wxLinkStatus.lastAttemptAt, $wxClock));
	let label = $derived(down ? `NWS DOWN ${age}` : `NWS ${age} stale`);
	let sentence = $derived(linkStateText($wxLinkStatus, $wxClock));

	// GPS pill occupies 178px..202px (24px tall) when shown; sit 4px below it,
	// or take its slot when it isn't shown. NextStopPill starts at 226px.
	let top = $derived($gpsStatus.enabled ? 206 : 178);
</script>

{#if visible}
	<button class="wx-link-pill" class:down onclick={() => openWeather('alerts')} style="top: {top}px" title={sentence} aria-label={sentence}>
		<span class="wx-link-dot" aria-hidden="true"></span>
		{label}
	</button>
{/if}

<style>
	.wx-link-pill {
		position: absolute;
		left: 10px;
		z-index: var(--z-toolbar);
		height: 24px;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 0 10px;
		background: rgba(10, 15, 30, 0.82);
		border: 1px solid var(--color-wx-watch);
		border-radius: var(--radius-full);
		color: var(--color-wx-watch);
		font-size: 11px;
		font-weight: 700;
		white-space: nowrap;
		cursor: pointer;
	}

	.wx-link-pill.down {
		border-color: var(--color-wx-warning);
		color: var(--color-wx-warning);
	}

	.wx-link-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
	}
</style>
