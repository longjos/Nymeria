<script lang="ts">
	import { conversationList } from '$lib/stores/messages';
	import { claimConversation, unclaimConversation } from '$lib/stores/messages';
	import { currentUser, canOperate } from '$lib/stores/session';
	import { timeAgo } from '$lib/utils';
	import { STATE_ACKED, STATE_FAILED, STATE_REJECTED } from '$lib/types';

	let {
		onSelectConvo,
		onNewMessage
	}: {
		onSelectConvo?: (callsign: string) => void;
		onNewMessage?: () => void;
	} = $props();

	let newTo = $state('');
	let showNew = $state(false);

	function startConversation() {
		const callsign = newTo.trim().toUpperCase();
		if (!callsign) return;
		showNew = false;
		newTo = '';
		onSelectConvo?.(callsign);
	}
</script>

<div class="convo-list">
	<div class="list-header">
		<span class="title">Messages</span>
		<button class="new-btn" onclick={() => (showNew = !showNew)}>
			{showNew ? 'Cancel' : 'New'}
		</button>
	</div>

	{#if showNew}
		<div class="new-convo">
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

	<div class="list-body">
		{#each $conversationList as convo (convo.callsign)}
			{@const lastMsg = convo.messages[convo.messages.length - 1]}
			<div class="convo-row" role="button" tabindex="0" onclick={() => onSelectConvo?.(convo.callsign)} onkeydown={(e) => { if (e.key === 'Enter') onSelectConvo?.(convo.callsign); }}>
				<div class="convo-info">
					<div class="convo-header">
						<span class="callsign">{convo.callsign}</span>
						{#if convo.unreadCount > 0}
							<span class="badge">{convo.unreadCount}</span>
						{/if}
						{#if convo.claimedName}
							<span class="claim-tag">{convo.claimedName}</span>
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
					{#if $canOperate && $currentUser}
						{#if convo.claimedBy === $currentUser.id}
							<button class="claim-btn unclaim" onmousedown={(e) => { e.stopPropagation(); unclaimConversation(convo.callsign); }}>
								Release
							</button>
						{:else if !convo.claimedBy}
							<button class="claim-btn" onmousedown={(e) => { e.stopPropagation(); claimConversation(convo.callsign, $currentUser.id, $currentUser.name); }}>
								Claim
							</button>
						{/if}
					{/if}
				</div>
			</div>
		{:else}
			<p class="empty">
				No conversations yet. Tap "New" to start one.
			</p>
		{/each}
	</div>
</div>

<style>
	.convo-list {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.list-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.title {
		font-weight: 600;
		font-size: 0.95rem;
	}

	.new-btn {
		padding: 0.3rem 0.65rem;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.new-convo {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.new-convo form {
		display: flex;
		gap: 0.5rem;
	}

	.new-convo input {
		flex: 1;
		padding: 0.4rem 0.6rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		font-family: monospace;
		text-transform: uppercase;
		outline: none;
	}

	.new-convo input:focus {
		border-color: var(--color-accent);
	}

	.new-convo button {
		padding: 0.4rem 0.75rem;
		background: var(--color-primary);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	.new-convo button:disabled {
		opacity: 0.5;
	}

	.list-body {
		flex: 1;
		overflow-y: auto;
	}

	.convo-row {
		display: flex;
		justify-content: space-between;
		width: 100%;
		padding: 0.65rem var(--space-md);
		border: none;
		border-bottom: 1px solid var(--color-primary);
		background: none;
		color: var(--color-text);
		cursor: pointer;
		text-align: left;
		transition: background var(--duration-fast);
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
		font-size: 0.9rem;
	}

	.badge {
		background: var(--color-accent);
		color: white;
		font-size: 0.6rem;
		font-weight: 700;
		padding: 0.1rem 0.35rem;
		border-radius: 10px;
	}

	.last-message {
		margin-top: 0.15rem;
		font-size: 0.78rem;
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
		gap: 0.2rem;
		flex-shrink: 0;
		margin-left: 0.5rem;
	}

	.time {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.state {
		font-size: 0.65rem;
	}

	.state.acked {
		color: var(--color-success);
	}

	.state.failed {
		color: var(--color-accent);
	}

	.claim-tag {
		font-size: 0.6rem;
		padding: 0.1rem 0.35rem;
		border-radius: var(--radius-sm);
		background: var(--color-primary);
		color: var(--color-text-muted);
		font-weight: 500;
	}

	.claim-btn {
		padding: 0.15rem 0.4rem;
		font-size: 0.65rem;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		background: none;
		color: var(--color-text-muted);
		cursor: pointer;
		transition: background var(--duration-fast), color var(--duration-fast);
	}

	.claim-btn:hover {
		background: var(--color-primary);
		color: var(--color-text);
	}

	.claim-btn.unclaim {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.claim-btn.unclaim:hover {
		background: var(--color-accent);
		color: white;
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
</style>
