<script lang="ts">
	import { gpsStatus, gpsFollow, gpsHasFix } from '$lib/stores/gps';

	let { oncenter }: { oncenter?: () => void } = $props();

	let hasFix = $derived($gpsHasFix);
	let following = $derived($gpsFollow);

	function handleClick() {
		if (!hasFix) return;
		const turningOn = !following;
		gpsFollow.set(turningOn);
		if (turningOn) oncenter?.();
	}
</script>

{#if $gpsStatus.enabled}
	<button
		type="button"
		class="gps-follow-btn"
		class:active={hasFix && following}
		class:waiting={!hasFix}
		disabled={!hasFix}
		aria-pressed={hasFix ? following : undefined}
		title={!hasFix ? 'Waiting for GPS fix' : following ? 'Following — drag the map to stop' : 'Follow my position'}
		onclick={handleClick}
	>
		<svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
			<circle cx="9" cy="9" r="3.5" />
			<line x1="9" y1="1" x2="9" y2="4" />
			<line x1="9" y1="14" x2="9" y2="17" />
			<line x1="1" y1="9" x2="4" y2="9" />
			<line x1="14" y1="9" x2="17" y2="9" />
		</svg>
	</button>
{/if}

<style>
	.gps-follow-btn {
		position: absolute;
		top: 130px;
		left: 10px;
		z-index: var(--z-toolbar);
		width: 40px;
		height: 40px;
		border-radius: 8px;
		border: 2px solid var(--color-text-muted);
		background: var(--color-surface);
		color: var(--color-text-muted);
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
		transition: background 0.15s, border-color 0.15s, color 0.15s;
	}

	.gps-follow-btn:hover:not(:disabled) {
		background: color-mix(in srgb, var(--color-text-muted) 15%, transparent);
	}

	.gps-follow-btn.waiting {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.gps-follow-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
		background: rgba(233, 69, 96, 0.15);
	}

	.gps-follow-btn.active:hover:not(:disabled) {
		background: rgba(233, 69, 96, 0.22);
	}
</style>
