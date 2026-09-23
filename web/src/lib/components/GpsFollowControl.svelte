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
		class="map-hud-btn gps-follow-btn"
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
	/* Size, surface and focus ring come from .map-hud-btn (app.css); the
	   page's HUD row places it. Only the follow states live here. */
	/* Dim the glyph, not the button: an opacity-faded button let map markers
	   show through it and read as part of the map. */
	.gps-follow-btn.waiting {
		color: color-mix(in srgb, var(--color-text-muted) 55%, transparent);
		cursor: not-allowed;
	}

	.gps-follow-btn.active {
		border-color: var(--color-accent);
		color: var(--color-accent);
		background: color-mix(in srgb, var(--color-accent) 15%, var(--color-surface));
	}
</style>
