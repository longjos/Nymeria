<script lang="ts">
	import { connectionState } from '$lib/stores/ui';

	let state = $derived($connectionState);

	let label = $derived.by(() => {
		switch (state) {
			case 'connected': return 'Connected';
			case 'disconnected': return 'Disconnected';
			case 'reconnecting': return 'Reconnecting';
		}
	});

	let dotColor = $derived.by(() => {
		switch (state) {
			case 'connected': return 'var(--color-connected)';
			case 'disconnected': return 'var(--color-disconnected)';
			case 'reconnecting': return 'var(--color-reconnecting)';
		}
	});
</script>

<div class="connection-pill">
	<span class="dot" class:pulse={state === 'reconnecting'} style="background: {dotColor}"></span>
	<span class="label">{label}</span>
</div>

<style>
	.connection-pill {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px 10px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-full);
		font-size: 0.75rem;
		color: var(--color-text-muted);
		pointer-events: auto;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.dot.pulse {
		animation: pulse 1.5s ease-in-out infinite;
	}

	@keyframes pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.3; }
	}
</style>
