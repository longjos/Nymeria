<script lang="ts">
	import type { Message } from '$lib/types';
	import { STATE_PENDING, STATE_SENT, STATE_ACKED, STATE_REJECTED, STATE_FAILED } from '$lib/types';

	let { message }: { message: Message } = $props();

	let stateLabel = $derived.by(() => {
		if (message.inbound) return '';
		switch (message.state) {
			case STATE_PENDING: return 'Sending...';
			case STATE_SENT: return 'Sent';
			case STATE_ACKED: return 'Delivered';
			case STATE_REJECTED: return 'Rejected';
			case STATE_FAILED: return 'Failed';
			default: return '';
		}
	});

	let stateClass = $derived.by(() => {
		if (message.inbound) return '';
		switch (message.state) {
			case STATE_ACKED: return 'acked';
			case STATE_REJECTED:
			case STATE_FAILED: return 'failed';
			default: return 'pending';
		}
	});

	let timeStr = $derived(
		new Date(message.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
	);
</script>

<div class="bubble" class:outbound={!message.inbound} class:inbound={message.inbound}>
	<div class="body">{message.body}</div>
	<div class="meta">
		<span class="time">{timeStr}</span>
		{#if stateLabel}
			<span class="state {stateClass}">{stateLabel}</span>
		{/if}
	</div>
</div>

<style>
	.bubble {
		max-width: 75%;
		padding: 0.5rem 0.75rem;
		border-radius: 12px;
		margin-bottom: 0.375rem;
		word-break: break-word;
	}

	.inbound {
		align-self: flex-start;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-bottom-left-radius: 4px;
	}

	.outbound {
		align-self: flex-end;
		background: var(--color-primary);
		border-bottom-right-radius: 4px;
	}

	.body {
		font-size: 0.9rem;
		line-height: 1.4;
	}

	.meta {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 0.25rem;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.state.acked {
		color: #4ade80;
	}

	.state.failed {
		color: var(--color-accent);
	}

	.state.pending {
		color: #f59e0b;
	}
</style>
