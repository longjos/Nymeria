<script lang="ts">
	// Pending/awaiting-reply list (spec §7): "r" opens this. Sorted
	// overdue-first then createdAt; Resolve/Cancel per row; "Add" is a
	// two-field inline form (summary, sentTo) — a pending-reply, not a
	// multi-field record, so it is allowed here per spec §5's exception list.
	import { api, ApiError } from '$lib/api';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { handoffOpen } from '$lib/stores/ride';
	import { showToast } from '$lib/stores/toast';
	import { currentUser } from '$lib/stores/session';

	let { onClose }: { onClose: () => void } = $props();

	let sorted = $derived(
		[...$handoffOpen].sort((a, b) => {
			const now = Date.now();
			const aOverdue = a.dueAt ? Date.parse(a.dueAt) < now : false;
			const bOverdue = b.dueAt ? Date.parse(b.dueAt) < now : false;
			if (aOverdue !== bOverdue) return aOverdue ? -1 : 1;
			return Date.parse(a.createdAt) - Date.parse(b.createdAt);
		})
	);

	let addOpen = $state(false);
	let summary = $state('');
	let sentTo = $state('');
	let submitting = $state(false);

	async function resolve(id: string, action: 'resolve' | 'cancel') {
		const netId = $activeNetId;
		if (!netId) return;
		try {
			await api.updateHandoffItem(netId, id, { action });
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Update failed', 'error');
		}
	}

	async function addItem() {
		const netId = $activeNetId;
		if (!netId || !summary.trim() || !sentTo.trim()) return;
		submitting = true;
		try {
			await api.addHandoffItem(netId, {
				kind: 'awaiting_reply',
				summary: summary.trim(),
				sentTo: sentTo.trim(),
				replyTo: $currentUser?.callsign || $currentUser?.name || 'NCS'
			});
			summary = '';
			sentTo = '';
			addOpen = false;
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not add item', 'error');
		} finally {
			submitting = false;
		}
	}

	let dialogEl = $state<HTMLElement | null>(null);
	$effect(() => {
		dialogEl?.querySelector<HTMLElement>('button')?.focus();
	});

	function trapTab(e: KeyboardEvent): void {
		if (e.key !== 'Tab' || !dialogEl) return;
		const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>('input, button:not([disabled])'));
		if (focusable.length === 0) return;
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	function handleKeydown(e: KeyboardEvent): void {
		if (e.key === 'Escape') onClose();
		else trapTab(e);
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="rpl-backdrop" role="presentation" onclick={onClose}>
	<div class="rpl" bind:this={dialogEl} role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="rpl-title" data-blocks-escape="true" onclick={(e) => e.stopPropagation()}>
		<div class="rpl-head">
			<h2 id="rpl-title" class="rpl-title">Pending</h2>
			<button class="rpl-close" onclick={onClose} aria-label="Close">&times;</button>
		</div>

		{#if sorted.length === 0}
			<p class="rpl-empty">Nothing pending.</p>
		{:else}
			<div class="rpl-list">
				{#each sorted as item (item.id)}
					{@const overdue = item.dueAt ? Date.parse(item.dueAt) < Date.now() : false}
					<div class="rpl-row" class:overdue>
						<div class="rpl-row-body">
							<strong>{item.summary}</strong>
							<span class="rpl-row-meta">Sent to {item.sentTo} · reply to {item.replyTo}</span>
						</div>
						<div class="rpl-row-actions">
							<button class="rpl-action" onclick={() => resolve(item.id, 'resolve')}>Resolve</button>
							<button class="rpl-action" onclick={() => resolve(item.id, 'cancel')}>Cancel</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		{#if addOpen}
			<div class="rpl-add">
				<input class="rpl-input" placeholder="What was asked" bind:value={summary} />
				<input class="rpl-input" placeholder="Sent to (callsign/tactical)" bind:value={sentTo} />
				<div class="rpl-add-actions">
					<button class="rpl-action" onclick={() => (addOpen = false)}>Cancel</button>
					<button class="rpl-action rpl-action-primary" disabled={submitting} aria-busy={submitting} onclick={addItem}>{submitting ? 'Adding…' : 'Add'}</button>
				</div>
			</div>
		{:else}
			<button class="rpl-add-btn" onclick={() => (addOpen = true)}>+ Add pending item</button>
		{/if}
	</div>
</div>

<style>
	.rpl-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.rpl {
		width: 100%;
		max-width: 420px;
		max-height: 80vh;
		overflow-y: auto;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.rpl-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.rpl-title {
		font-size: 0.95rem;
		font-weight: 600;
	}

	.rpl-close {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		cursor: pointer;
		min-width: 32px;
		min-height: 32px;
	}

	.rpl-empty {
		font-size: var(--ride-t-body);
		color: var(--color-text-muted);
	}

	.rpl-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.rpl-row {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm);
		border-radius: var(--radius-sm);
		background: var(--color-bg);
	}

	.rpl-row.overdue {
		border-left: 3px solid var(--color-warning);
	}

	.rpl-row-body {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
		font-size: var(--ride-t-body);
	}

	.rpl-row-meta {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
	}

	.rpl-row-actions {
		display: flex;
		gap: var(--space-xs);
		flex-shrink: 0;
	}

	.rpl-action {
		min-height: 44px;
		padding: 0 var(--space-sm);
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: var(--ride-t-label);
		font-weight: 600;
		cursor: pointer;
	}

	.rpl-action-primary {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: var(--color-on-accent);
	}

	.rpl-add {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		border-top: 1px solid var(--color-primary);
		padding-top: var(--space-sm);
	}

	.rpl-input {
		min-height: 40px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: 0 var(--space-sm);
		font: inherit;
	}

	.rpl-add-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
	}

	.rpl-add-btn {
		min-height: 40px;
		background: none;
		border: 1px dashed var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-accent);
		font-weight: 600;
		cursor: pointer;
	}
</style>
