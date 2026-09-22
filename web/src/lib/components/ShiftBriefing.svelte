<script lang="ts">
	// The shift-relief briefing (spec §7): a frozen snapshot, the full pending
	// list grouped by owner, open items by tier, and the read-back confirm
	// ("I have the net") that is the deliberate steal from I-PASS — the
	// incoming operator restates before taking over.
	import { api, ApiError } from '$lib/api';
	import { activeNetId } from '$lib/stores/netcontrol';
	import { wxIsNcs } from '$lib/stores/wxAlerts';
	import { canOperate, currentUser } from '$lib/stores/session';
	import { showToast } from '$lib/stores/toast';
	import type { ShiftBriefing as ShiftBriefingT, HandoffItem } from '$lib/types';
	import { clock } from '$lib/wxAlertTime';

	let { onClose }: { onClose: () => void } = $props();

	let brief = $state<ShiftBriefingT | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let takingOver = $state(false);

	$effect(() => {
		const netId = $activeNetId;
		if (!netId) return;
		let cancelled = false;
		(async () => {
			try {
				const b = await api.rideBriefing(netId);
				if (!cancelled) brief = b;
			} catch (e) {
				if (!cancelled) error = e instanceof ApiError ? e.message : 'Could not load the briefing.';
			} finally {
				if (!cancelled) loading = false;
			}
		})();
		return () => {
			cancelled = true;
		};
	});

	async function iHaveTheNet(): Promise<void> {
		const netId = $activeNetId;
		const user = $currentUser;
		if (!netId || !user) return;
		takingOver = true;
		try {
			await api.transferNCS(netId, user.callsign || user.name, user.id);
			await api.ackHandoff(netId);
			showToast('You have the net — briefing acknowledged', 'success');
			onClose();
		} catch (e) {
			showToast(e instanceof ApiError ? e.message : 'Could not take the net', 'error');
		} finally {
			takingOver = false;
		}
	}

	let dialogEl = $state<HTMLElement | null>(null);
	$effect(() => {
		dialogEl?.querySelector<HTMLElement>('button')?.focus();
	});

	function trapTab(e: KeyboardEvent): void {
		if (e.key !== 'Tab' || !dialogEl) return;
		const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>('button:not([disabled])'));
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

	let openByOwner = $derived.by(() => {
		const map = new Map<string, HandoffItem[]>();
		for (const item of brief?.openItems ?? []) {
			const key = item.replyTo || 'NCS';
			const arr = map.get(key) ?? [];
			arr.push(item);
			map.set(key, arr);
		}
		return map;
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="sb-backdrop" role="presentation" onclick={onClose}>
	<div class="sb" bind:this={dialogEl} role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="sb-title" data-blocks-escape="true" onclick={(e) => e.stopPropagation()}>
		<div class="sb-head">
			<h2 id="sb-title" class="sb-title">Shift briefing</h2>
			<button class="sb-close" onclick={onClose} aria-label="Close">&times;</button>
		</div>

		{#if loading}
			<p class="sb-loading">Loading…</p>
		{:else if error}
			<p class="sb-error">{error}</p>
		{:else if brief}
			<p class="sb-generated">Snapshot at {clock(brief.generatedAt)}{#if brief.ncsSince} · NCS since {clock(brief.ncsSince)}{/if}</p>

			{#if brief.sections.length > 0}
				{#each brief.sections as section (section.key)}
					<div class="sb-section">
						<h3 class="sb-section-title">{section.title}</h3>
						{#each section.lines as line, i (i)}
							<div class="sb-line sb-severity-{line.severity}">
								<span class="sb-line-label">{line.label}</span>
								<span class="sb-line-value">{line.value}</span>
							</div>
						{/each}
					</div>
				{/each}
			{/if}

			<div class="sb-section">
				<h3 class="sb-section-title">Pending, by owner</h3>
				{#if openByOwner.size === 0}
					<p class="sb-empty">Nothing pending.</p>
				{:else}
					{#each [...openByOwner.entries()] as [owner, items] (owner)}
						<div class="sb-owner-group">
							<span class="sb-owner">{owner}</span>
							{#each items as item (item.id)}
								<div class="sb-line">
									<span class="sb-line-label">{item.summary}</span>
									<span class="sb-line-value">{item.sentTo}</span>
								</div>
							{/each}
						</div>
					{/each}
				{/if}
			</div>

			{#if brief.awaitingReplies.length > 0}
				<div class="sb-section">
					<h3 class="sb-section-title">Awaiting reply</h3>
					{#each brief.awaitingReplies as ar (ar.messageId)}
						<div class="sb-line">
							<span class="sb-line-label">{ar.to}</span>
							<span class="sb-line-value">{ar.body}</span>
						</div>
					{/each}
				</div>
			{/if}

			<div class="sb-section">
				<h3 class="sb-section-title">Accounting</h3>
				<div class="sb-line">
					<span class="sb-line-label">Supported / unsupported</span>
					<span class="sb-line-value">{brief.accounting.counts.supported} / {brief.accounting.counts.unsupported}</span>
				</div>
				<div class="sb-line">
					<span class="sb-line-label">Sweep</span>
					<span class="sb-line-value">{brief.accounting.sweepDetail}</span>
				</div>
			</div>

			<div class="sb-section">
				<h3 class="sb-section-title">Close-out</h3>
				{#each brief.closeout.items as item (item.key)}
					<div class="sb-line">
						<span class="sb-line-label">{item.done ? '✓' : '○'} {item.label}</span>
						<span class="sb-line-value">{item.total > 0 ? `${item.count}/${item.total}` : ''}</span>
					</div>
				{/each}
			</div>

			{#if !$wxIsNcs && $canOperate}
				<button class="sb-take-net" disabled={takingOver} onclick={iHaveTheNet}>
					{takingOver ? 'Taking over…' : 'I have the net'}
				</button>
			{/if}
		{/if}
	</div>
</div>

<style>
	.sb-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.sb {
		width: 100%;
		max-width: 480px;
		max-height: 85vh;
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

	.sb-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.sb-title {
		font-size: 1.05rem;
		font-weight: 700;
	}

	.sb-close {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		cursor: pointer;
		min-width: 32px;
		min-height: 32px;
	}

	.sb-generated {
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.sb-loading,
	.sb-error,
	.sb-empty {
		font-size: 0.82rem;
		color: var(--color-text-muted);
	}

	.sb-error {
		color: var(--color-error-text);
	}

	.sb-section {
		border-top: 1px solid var(--color-primary);
		padding-top: var(--space-sm);
	}

	.sb-section-title {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-muted);
		margin-bottom: 4px;
	}

	.sb-owner-group {
		margin-bottom: var(--space-xs);
	}

	.sb-owner {
		font-size: 0.72rem;
		font-weight: 700;
		color: var(--color-accent);
	}

	.sb-line {
		display: flex;
		justify-content: space-between;
		gap: var(--space-sm);
		font-size: 0.8rem;
		padding: 2px 0;
	}

	.sb-severity-urgent {
		border-left: 3px solid var(--color-error);
		padding-left: 6px;
	}

	.sb-line-label {
		color: var(--color-text);
	}

	.sb-line-value {
		color: var(--color-text-muted);
		text-align: right;
	}

	.sb-take-net {
		min-height: 48px;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-md);
		color: var(--color-on-accent);
		font-weight: 800;
		font-size: 0.95rem;
		cursor: pointer;
	}

	.sb-take-net:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
