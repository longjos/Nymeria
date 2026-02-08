<script lang="ts">
	import { sendMessage } from '$lib/stores/messages';

	let { to }: { to: string } = $props();
	let body = $state('');
	let sending = $state(false);
	let error = $state('');

	async function handleSubmit(e: Event) {
		e.preventDefault();
		const text = body.trim();
		if (!text || !to) return;

		sending = true;
		error = '';
		try {
			await sendMessage(to, text);
			body = '';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Send failed';
		} finally {
			sending = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleSubmit(e);
		}
	}
</script>

<form class="compose" onsubmit={handleSubmit}>
	{#if error}
		<div class="error">{error}</div>
	{/if}
	<div class="input-row">
		<input
			type="text"
			bind:value={body}
			placeholder="Type a message..."
			disabled={sending}
			maxlength="67"
			onkeydown={handleKeydown}
		/>
		<button type="submit" disabled={sending || !body.trim()}>
			{sending ? '...' : 'Send'}
		</button>
	</div>
	<div class="hint">{body.length}/67 characters (APRS limit)</div>
</form>

<style>
	.compose {
		padding: 0.75rem;
		border-top: 1px solid var(--color-primary);
		background: var(--color-surface);
	}

	.input-row {
		display: flex;
		gap: 0.5rem;
	}

	input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: 20px;
		color: var(--color-text);
		font-size: 0.9rem;
		outline: none;
	}

	input:focus {
		border-color: var(--color-accent);
	}

	button {
		padding: 0.5rem 1rem;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: 20px;
		font-weight: 600;
		font-size: 0.85rem;
		cursor: pointer;
	}

	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.error {
		color: var(--color-accent);
		font-size: 0.8rem;
		margin-bottom: 0.375rem;
	}

	.hint {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		margin-top: 0.25rem;
		text-align: right;
	}
</style>
