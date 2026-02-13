<script lang="ts">
	import { toasts, dismissToast } from '$lib/stores/toast';
	import { fly, fade } from 'svelte/transition';
</script>

{#if $toasts.length > 0}
	<div class="toast-container" role="status" aria-live="polite">
		{#each $toasts as toast (toast.id)}
			<div
				class="toast toast-{toast.type}"
				transition:fly={{ x: 80, duration: 250, easing: t => t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2 }}
			>
				<span class="toast-icon">
					{#if toast.type === 'success'}&#10003;{:else if toast.type === 'error'}&#10007;{:else}&#8505;{/if}
				</span>
				<span class="toast-message">{toast.message}</span>
				<button
					class="toast-dismiss"
					onclick={() => dismissToast(toast.id)}
					aria-label="Dismiss notification"
				>&times;</button>
			</div>
		{/each}
	</div>
{/if}

<style>
	.toast-container {
		position: fixed;
		bottom: var(--space-lg);
		right: var(--space-lg);
		z-index: 9999;
		display: flex;
		flex-direction: column-reverse;
		gap: var(--space-sm);
		pointer-events: none;
		max-width: 380px;
	}

	.toast {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		border-radius: var(--radius-md);
		background: var(--color-surface);
		color: var(--color-text);
		box-shadow: var(--shadow-lg);
		pointer-events: auto;
		font-size: 0.875rem;
		line-height: 1.4;
		border-left: 3px solid;
	}

	.toast-success { border-left-color: #22c55e; }
	.toast-error   { border-left-color: #ef4444; }
	.toast-info    { border-left-color: #3b82f6; }

	.toast-icon {
		flex-shrink: 0;
		width: 18px;
		text-align: center;
		font-weight: 700;
	}

	.toast-success .toast-icon { color: #22c55e; }
	.toast-error   .toast-icon { color: #ef4444; }
	.toast-info    .toast-icon { color: #3b82f6; }

	.toast-message {
		flex: 1;
		min-width: 0;
	}

	.toast-dismiss {
		flex-shrink: 0;
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 1.125rem;
		line-height: 1;
		padding: 2px 4px;
		border-radius: var(--radius-sm);
	}

	.toast-dismiss:hover {
		color: var(--color-text);
		background: rgba(255, 255, 255, 0.1);
	}

	@media (max-width: 768px) {
		.toast-container {
			right: var(--space-md);
			left: var(--space-md);
			bottom: calc(var(--sheet-peek) + var(--space-md));
			max-width: none;
		}
	}
</style>
