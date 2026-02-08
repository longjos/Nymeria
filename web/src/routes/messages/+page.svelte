<script lang="ts">
	import { onMount } from 'svelte';
	import { conversationList, initMessageStore } from '$lib/stores/messages';
	import { initStationStore } from '$lib/stores/stations';
	import { timeAgo } from '$lib/utils';
	import { STATE_ACKED, STATE_FAILED, STATE_REJECTED } from '$lib/types';

	let newTo = $state('');
	let showNew = $state(false);

	onMount(() => {
		initStationStore();
		initMessageStore();
	});

	function startConversation() {
		const callsign = newTo.trim().toUpperCase();
		if (!callsign) return;
		window.location.href = `/messages/${callsign}`;
	}
</script>

<svelte:head>
	<title>Messages - Nymeria</title>
</svelte:head>

<div class="messages-page">
	<div class="page-header">
		<h2>Messages</h2>
		<button class="new-btn" onclick={() => (showNew = !showNew)}>
			{showNew ? 'Cancel' : 'New Message'}
		</button>
	</div>

	{#if showNew}
		<div class="new-conversation">
			<form onsubmit={(e) => { e.preventDefault(); startConversation(); }}>
				<input
					type="text"
					placeholder="Enter callsign..."
					bind:value={newTo}
				/>
				<button type="submit" disabled={!newTo.trim()}>Go</button>
			</form>
		</div>
	{/if}

	<div class="conversation-list">
		{#each $conversationList as convo (convo.callsign)}
			{@const lastMsg = convo.messages[convo.messages.length - 1]}
			<a href="/messages/{convo.callsign}" class="convo-row">
				<div class="convo-info">
					<div class="convo-header">
						<span class="callsign">{convo.callsign}</span>
						{#if convo.unreadCount > 0}
							<span class="badge">{convo.unreadCount}</span>
						{/if}
					</div>
					{#if lastMsg}
						<div class="last-message">
							{#if !lastMsg.inbound}
								<span class="you">You: </span>
							{/if}
							<span class="preview">{lastMsg.body}</span>
						</div>
					{/if}
				</div>
				<div class="convo-meta">
					<span class="time">{timeAgo(convo.lastActive)}</span>
					{#if lastMsg && !lastMsg.inbound}
						{#if lastMsg.state === STATE_ACKED}
							<span class="state acked">Delivered</span>
						{:else if lastMsg.state === STATE_FAILED || lastMsg.state === STATE_REJECTED}
							<span class="state failed">Failed</span>
						{/if}
					{/if}
				</div>
			</a>
		{:else}
			<p class="empty">
				No conversations yet. Click "New Message" to start one.
			</p>
		{/each}
	</div>
</div>

<style>
	.messages-page {
		max-width: 600px;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	h2 {
		font-size: 1.25rem;
	}

	.new-btn {
		padding: 0.4rem 0.75rem;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.new-conversation {
		margin-bottom: 1rem;
	}

	.new-conversation form {
		display: flex;
		gap: 0.5rem;
	}

	.new-conversation input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: 6px;
		color: var(--color-text);
		font-size: 0.9rem;
		font-family: monospace;
		text-transform: uppercase;
		outline: none;
	}

	.new-conversation input:focus {
		border-color: var(--color-accent);
	}

	.new-conversation button {
		padding: 0.5rem 1rem;
		background: var(--color-primary);
		color: var(--color-text);
		border: none;
		border-radius: 6px;
		cursor: pointer;
	}

	.new-conversation button:disabled {
		opacity: 0.5;
	}

	.conversation-list {
		display: flex;
		flex-direction: column;
	}

	.convo-row {
		display: flex;
		justify-content: space-between;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--color-primary);
		text-decoration: none;
		color: var(--color-text);
		transition: background 0.1s;
	}

	.convo-row:first-child {
		border-top: 1px solid var(--color-primary);
	}

	.convo-row:hover {
		background: var(--color-surface);
	}

	.convo-info {
		min-width: 0;
		flex: 1;
	}

	.convo-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.callsign {
		font-weight: 600;
		font-family: monospace;
		font-size: 0.95rem;
	}

	.badge {
		background: var(--color-accent);
		color: white;
		font-size: 0.65rem;
		font-weight: 700;
		padding: 0.1rem 0.4rem;
		border-radius: 10px;
	}

	.last-message {
		margin-top: 0.2rem;
		font-size: 0.8rem;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.you {
		color: var(--color-text);
	}

	.convo-meta {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.25rem;
		flex-shrink: 0;
		margin-left: 0.75rem;
	}

	.time {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.state {
		font-size: 0.7rem;
	}

	.state.acked {
		color: #4ade80;
	}

	.state.failed {
		color: var(--color-accent);
	}

	.empty {
		padding: 3rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
	}
</style>
