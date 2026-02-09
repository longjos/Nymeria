<script lang="ts">
	import { bulletinList, announcements, regularBulletins } from '$lib/stores/bulletins';
	import { canOperate } from '$lib/stores/session';
	import { sendMessage } from '$lib/stores/messages';
	import { timeAgo } from '$lib/utils';

	let showCompose = $state(false);
	let blnNumber = $state('0');
	let blnBody = $state('');
	let sending = $state(false);

	const MAX_BODY = 67;

	let charsLeft = $derived(MAX_BODY - blnBody.length);
	let canSend = $derived(blnBody.trim().length > 0 && charsLeft >= 0 && !sending);

	async function handleSend() {
		if (!canSend) return;
		sending = true;
		try {
			await sendMessage(`BLN${blnNumber}`, blnBody.trim());
			blnBody = '';
			showCompose = false;
		} catch {
			// silent
		} finally {
			sending = false;
		}
	}
</script>

<div class="bulletin-panel">
	<div class="list-header">
		<span class="title">
			Bulletin Board
			{#if $bulletinList.length > 0}
				<span class="count">{$bulletinList.length}</span>
			{/if}
		</span>
		{#if $canOperate}
			<button class="post-btn" onclick={() => (showCompose = !showCompose)}>
				{showCompose ? 'Cancel' : 'Post'}
			</button>
		{/if}
	</div>

	{#if showCompose}
		<div class="compose">
			<form onsubmit={(e) => { e.preventDefault(); handleSend(); }}>
				<div class="compose-row">
					<label class="bln-label">
						BLN
						<select bind:value={blnNumber}>
							{#each Array.from({length: 10}, (_, i) => String(i)) as n}
								<option value={n}>{n}</option>
							{/each}
						</select>
					</label>
					<input
						type="text"
						placeholder="Bulletin text..."
						bind:value={blnBody}
						maxlength={MAX_BODY}
					/>
				</div>
				<div class="compose-footer">
					<span class="char-count" class:warn={charsLeft < 10}>{charsLeft}</span>
					<button type="submit" disabled={!canSend}>
						{sending ? 'Sending...' : 'Send'}
					</button>
				</div>
			</form>
		</div>
	{/if}

	<div class="list-body">
		{#if $announcements.length > 0}
			<div class="section-label">Announcements</div>
			{#each $announcements as b (b.id)}
				<div class="bulletin-row announcement">
					<div class="bln-badge ann">ANN</div>
					<div class="bln-content">
						<div class="bln-header">
							<span class="bln-from">{b.from}</span>
							<span class="bln-time">{timeAgo(b.timestamp)}</span>
						</div>
						<div class="bln-body">{b.body}</div>
					</div>
				</div>
			{/each}
		{/if}

		{#if $regularBulletins.length > 0}
			{#if $announcements.length > 0}
				<div class="section-label">Bulletins</div>
			{/if}
			{#each $regularBulletins as b (b.id)}
				<div class="bulletin-row">
					<div class="bln-badge">{b.bulletinId.replace('BLN', '')}</div>
					<div class="bln-content">
						<div class="bln-header">
							<span class="bln-from">{b.from}</span>
							<span class="bln-time">{timeAgo(b.timestamp)}</span>
						</div>
						<div class="bln-body">{b.body}</div>
					</div>
				</div>
			{/each}
		{/if}

		{#if $bulletinList.length === 0}
			<p class="empty">No bulletins received</p>
		{/if}
	</div>
</div>

<style>
	.bulletin-panel {
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
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.count {
		font-size: 0.7rem;
		font-weight: 700;
		padding: 0.1rem 0.4rem;
		border-radius: 10px;
		background: var(--color-primary);
		color: var(--color-text-muted);
	}

	.post-btn {
		padding: 0.3rem 0.65rem;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.compose {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.compose-row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.bln-label {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.8rem;
		font-weight: 600;
		font-family: monospace;
		color: var(--color-text);
		white-space: nowrap;
	}

	.bln-label select {
		padding: 0.3rem 0.25rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		font-family: monospace;
		outline: none;
	}

	.bln-label select:focus {
		border-color: var(--color-accent);
	}

	.compose input {
		flex: 1;
		padding: 0.4rem 0.6rem;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		outline: none;
	}

	.compose input:focus {
		border-color: var(--color-accent);
	}

	.compose-footer {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-top: 0.4rem;
	}

	.char-count {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.char-count.warn {
		color: var(--color-accent);
		font-weight: 600;
	}

	.compose-footer button {
		padding: 0.35rem 0.75rem;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.compose-footer button:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.list-body {
		flex: 1;
		overflow-y: auto;
	}

	.section-label {
		padding: 0.4rem var(--space-md);
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
		background: var(--color-surface);
		border-bottom: 1px solid var(--color-primary);
	}

	.bulletin-row {
		display: flex;
		gap: 0.6rem;
		padding: 0.6rem var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		align-items: flex-start;
	}

	.bulletin-row.announcement {
		background: color-mix(in srgb, var(--color-accent) 5%, transparent);
	}

	.bln-badge {
		flex-shrink: 0;
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		background: var(--color-primary);
		color: var(--color-text);
		font-size: 0.75rem;
		font-weight: 700;
		font-family: monospace;
	}

	.bln-badge.ann {
		background: var(--color-accent);
		color: white;
		font-size: 0.55rem;
		letter-spacing: 0.02em;
	}

	.bln-content {
		flex: 1;
		min-width: 0;
	}

	.bln-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	.bln-from {
		font-family: monospace;
		font-weight: 600;
		font-size: 0.8rem;
	}

	.bln-time {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.bln-body {
		margin-top: 0.15rem;
		font-size: 0.85rem;
		color: var(--color-text);
		word-break: break-word;
	}

	.empty {
		padding: 2rem 1rem;
		text-align: center;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
</style>
