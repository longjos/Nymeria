<script lang="ts">
	import { api, ApiError } from '$lib/api';
	import { canOperate } from '$lib/stores/session';
	import { annotationBatches } from '$lib/stores/annotations';
	import { showToast } from '$lib/stores/toast';
	import { timeAgo } from '$lib/utils';
	import type { AnnotationBatch, BulkDeleteResult, TransmittingMember } from '$lib/types';

	let {
		batch,
		netClosed = false,
		netName = '',
		returnFocusOnSuccess = null,
		onClose,
	}: {
		batch: AnnotationBatch;
		netClosed?: boolean;
		netName?: string;
		/** Focused after a successful removal, because the invoking button is gone. */
		returnFocusOnSuccess?: HTMLElement | null;
		onClose: () => void;
	} = $props();

	let dialogEl = $state<HTMLDivElement | null>(null);
	let cancelEl = $state<HTMLButtonElement | null>(null);

	let includeMissionLinked = $state(false);
	let pending = $state(false);
	let errorMsg = $state<string | null>(null);
	let transmitting = $state<TransmittingMember[]>([]);

	/** Set once the dialog has decided to close, so the race guard stays quiet. */
	let closing = false;

	// The prop is a snapshot; the store is the truth while the dialog is open.
	let live = $derived($annotationBatches.find((b) => b.id === batch.id) ?? null);
	let count = $derived(live?.count ?? batch.count);
	let missionLinkedCount = $derived(live?.missionLinkedCount ?? batch.missionLinkedCount);
	let checkpointCount = $derived(live?.checkpointCount ?? batch.checkpointCount);
	let label = $derived(live?.label ?? batch.label);
	let effectiveCount = $derived(includeMissionLinked ? count : count - missionLinkedCount);
	let blockedByTransmission = $derived(transmitting.length > 0 && !$canOperate);

	// Another operator removed the set while this dialog was open. The dialog is
	// only ever opened from a batch that is already in the store, so "gone from
	// the store" is unambiguous even when it was the only set.
	$effect(() => {
		if (closing) return;
		if (!live) {
			finish(false);
			showToast('That imported set was already removed.', 'info');
		}
	});

	// Focus the dialog, isolate the rest of the page, and lock background scroll.
	$effect(() => {
		const node = dialogEl;
		if (!node) return;

		const previouslyFocused = document.activeElement as HTMLElement | null;
		const hidden: HTMLElement[] = [];
		for (const child of Array.from(document.body.children)) {
			const el = child as HTMLElement;
			if (el.contains(node) || el.tagName === 'SCRIPT' || el.tagName === 'STYLE') continue;
			if (el.hasAttribute('aria-hidden')) continue;
			el.setAttribute('aria-hidden', 'true');
			el.setAttribute('inert', '');
			hidden.push(el);
		}

		const prevOverflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';

		// Cancel, never the danger button, takes initial focus.
		cancelEl?.focus();

		return () => {
			for (const el of hidden) {
				el.removeAttribute('aria-hidden');
				el.removeAttribute('inert');
			}
			document.body.style.overflow = prevOverflow;
			const target = closing && returnFocusOnSuccess?.isConnected
				? returnFocusOnSuccess
				: previouslyFocused;
			if (target?.isConnected) target.focus();
		};
	});

	function finish(succeeded: boolean) {
		closing = succeeded;
		onClose();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			finish(false);
			return;
		}
		if (e.key !== 'Tab' || !dialogEl) return;
		const focusables = Array.from(
			dialogEl.querySelectorAll<HTMLElement>(
				'button:not([disabled]), input:not([disabled]), [href], [tabindex]:not([tabindex="-1"])'
			)
		).filter((el) => el.offsetParent !== null || el === document.activeElement);
		if (focusables.length === 0) return;
		const first = focusables[0];
		const last = focusables[focusables.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	function summarize(res: BulkDeleteResult, total: number): string {
		let msg = `Removed ${res.deletedCount} of ${total} items from ${label}`;
		if (res.skippedMissionLinked?.length) {
			msg += ` — ${res.skippedMissionLinked.length} mission-linked item${res.skippedMissionLinked.length === 1 ? '' : 's'} kept`;
		}
		if (res.killedObjects > 0) {
			msg += ` — ${res.killedObjects} APRS object${res.killedObjects === 1 ? '' : 's'} killed`;
		}
		return msg;
	}

	async function undo(token: string) {
		try {
			const res = await api.undoDeleteAnnotations(token);
			showToast(`Restored ${res.count} items. APRS transmission was not resumed.`, 'success', 6000);
		} catch (e) {
			if (e instanceof ApiError && e.status === 410) {
				showToast('Undo window expired', 'error');
				return;
			}
			showToast(e instanceof Error ? e.message : 'Undo failed', 'error');
		}
	}

	async function remove(stopTransmit: boolean) {
		if (pending) return;
		pending = true;
		errorMsg = null;
		const total = count;
		try {
			const res = await api.bulkDeleteAnnotations({
				batchId: batch.id,
				includeMissionLinked,
				stopTransmit,
			});
			finish(true);
			showToast(
				summarize(res, total),
				'success',
				12000,
				res.undoToken ? { label: 'Undo', run: () => undo(res.undoToken as string) } : undefined
			);
		} catch (e) {
			if (e instanceof ApiError) {
				if (e.status === 404) {
					finish(false);
					showToast('That imported set was already removed.', 'info');
					return;
				}
				if (e.status === 409) {
					transmitting = Array.isArray(e.body.transmitting)
						? (e.body.transmitting as TransmittingMember[])
						: [];
					errorMsg = transmitting.length ? null : e.message;
					return;
				}
			}
			errorMsg = e instanceof Error ? e.message : 'Removal failed';
		} finally {
			pending = false;
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="batch-backdrop"
	onmousedown={(e) => { if (e.target === e.currentTarget) finish(false); }}
>
	<div
		class="batch-dialog"
		bind:this={dialogEl}
		role="dialog"
		aria-modal="true"
		aria-labelledby="batch-remove-title"
		aria-describedby="batch-remove-warnings"
	>
		<div class="grab-handle" aria-hidden="true"></div>

		<div class="batch-header">
			<span class="batch-title" id="batch-remove-title">Remove imported set</span>
			<button class="batch-close" onclick={() => finish(false)} aria-label="Close">&times;</button>
		</div>

		<div class="batch-body">
			<div class="batch-identity">
				<span class="identity-dot" aria-hidden="true"></span>
				<div class="identity-text">
					<span class="identity-label" title={label}>{label}</span>
					<span class="identity-sub">
						{count} items &middot; imported {timeAgo(batch.createdAt)}{netName ? ` · ${netName}` : ''}
					</span>
				</div>
			</div>

			<ul class="warn-list" id="batch-remove-warnings">
				{#if missionLinkedCount > 0}
					<li class="warn-row">
						<span class="warn-icon warn-mission" aria-hidden="true">!</span>
						<div class="warn-text">
							<span>{missionLinkedCount} item{missionLinkedCount === 1 ? ' is' : 's are'} linked to missions.</span>
							<label class="warn-check">
								<input type="checkbox" bind:checked={includeMissionLinked} />
								Also remove the {missionLinkedCount} mission-linked item{missionLinkedCount === 1 ? '' : 's'}
							</label>
						</div>
					</li>
				{/if}
				{#if checkpointCount > 0}
					<li class="warn-row">
						<span class="warn-icon" aria-hidden="true">#</span>
						<div class="warn-text">
							<span>{checkpointCount} checkpoint{checkpointCount === 1 ? '' : 's'} will lose {checkpointCount === 1 ? 'its sequence number' : 'their sequence numbers'}. Logged passages stay in the net timeline.</span>
						</div>
					</li>
				{/if}
				{#if netClosed}
					<li class="warn-row">
						<span class="warn-icon" aria-hidden="true">&#9633;</span>
						<div class="warn-text">
							<span>This net is closed. Removed locations disappear from its location history.</span>
						</div>
					</li>
				{/if}
				{#if transmitting.length > 0}
					<li class="warn-row warn-row-tx" id="batch-remove-tx">
						<span class="warn-icon warn-tx" aria-hidden="true">&#9679;</span>
						<div class="warn-text">
							<span>{transmitting.length} item{transmitting.length === 1 ? ' is' : 's are'} transmitting as APRS objects. Removing them sends a kill packet.</span>
							<span class="warn-names">{transmitting.map((t) => t.label).join(', ')}</span>
							{#if !$canOperate}
								<span class="warn-blocked">An operator must stop transmission before this set can be removed.</span>
							{/if}
						</div>
					</li>
				{/if}
			</ul>

			<p class="batch-footnote">You'll have 60 seconds to undo.</p>

			{#if errorMsg}
				<div class="batch-error" role="alert">{errorMsg}</div>
			{/if}
		</div>

		<div class="batch-footer">
			<button class="batch-btn" bind:this={cancelEl} onclick={() => finish(false)} disabled={pending}>
				Cancel
			</button>
			{#if transmitting.length > 0}
				<button
					class="batch-btn batch-btn-danger"
					onclick={() => remove(true)}
					disabled={pending || blockedByTransmission}
					aria-busy={pending}
					aria-describedby={blockedByTransmission ? 'batch-remove-tx' : undefined}
				>
					{pending ? 'Removing…' : `Stop transmission and remove ${effectiveCount} items`}
				</button>
			{:else}
				<button
					class="batch-btn batch-btn-danger"
					onclick={() => remove(false)}
					disabled={pending || effectiveCount <= 0}
					aria-busy={pending}
				>
					{pending ? 'Removing…' : `Remove ${effectiveCount} item${effectiveCount === 1 ? '' : 's'}`}
				</button>
			{/if}
		</div>
	</div>
</div>

<style>
	.batch-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: rgba(0, 0, 0, 0.6);
		display: flex;
		justify-content: center;
		align-items: center;
		padding: var(--space-lg);
	}

	.batch-dialog {
		width: min(440px, 92vw);
		max-height: min(80vh, 620px);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.grab-handle {
		display: none;
	}

	.batch-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-md);
		border-bottom: 1px solid rgba(255, 255, 255, 0.06);
		flex-shrink: 0;
	}

	.batch-title {
		font-weight: 600;
		font-size: 0.9rem;
	}

	.batch-close {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		line-height: 1;
		cursor: pointer;
		padding: 4px 8px;
		border-radius: 4px;
	}

	.batch-close:hover {
		color: var(--color-text);
	}

	.batch-body {
		flex: 1;
		overflow-y: auto;
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	/* Identity */
	.batch-identity {
		display: flex;
		align-items: flex-start;
		gap: var(--space-sm);
		min-width: 0;
	}

	.identity-dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		margin-top: 5px;
		border-radius: 50%;
		background: var(--color-accent);
	}

	.identity-text {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.identity-label {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.identity-sub {
		font-size: 0.6875rem;
		color: var(--color-text-muted);
	}

	/* Consequences */
	.warn-list {
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		margin: 0;
		padding: 0;
	}

	.warn-list:empty {
		display: none;
	}

	.warn-row {
		display: flex;
		align-items: flex-start;
		gap: var(--space-sm);
		padding: var(--space-sm);
		background: rgba(255, 255, 255, 0.03);
		border-radius: var(--radius-sm);
	}

	.warn-row-tx {
		background: rgba(231, 76, 60, 0.08);
		border: 1px solid rgba(231, 76, 60, 0.35);
	}

	.warn-icon {
		flex-shrink: 0;
		width: 18px;
		height: 18px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		background: rgba(255, 255, 255, 0.08);
		color: var(--color-text-muted);
		font-size: 0.6875rem;
		font-weight: 700;
	}

	.warn-mission {
		background: rgba(245, 158, 11, 0.18);
		color: var(--color-warning);
	}

	.warn-tx {
		background: rgba(231, 76, 60, 0.2);
		color: var(--color-error-text);
	}

	.warn-text {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 0.8125rem;
		line-height: 1.4;
		color: var(--color-text);
		min-width: 0;
	}

	.warn-names {
		font-size: 0.6875rem;
		color: var(--color-text-muted);
		word-break: break-word;
	}

	.warn-blocked {
		font-size: 0.75rem;
		color: var(--color-warning);
	}

	.warn-check {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		font-size: 0.75rem;
		color: var(--color-text-muted);
		cursor: pointer;
		min-height: 32px;
	}

	.warn-check input {
		width: 16px;
		height: 16px;
		accent-color: var(--color-accent);
		cursor: pointer;
	}

	.batch-footnote {
		font-size: 0.6875rem;
		color: var(--color-text-muted);
		margin: 0;
	}

	.batch-error {
		font-size: 0.75rem;
		line-height: 1.4;
		color: var(--color-error-text);
		padding: var(--space-sm);
		background: rgba(231, 76, 60, 0.1);
		border-radius: var(--radius-sm);
	}

	/* Footer */
	.batch-footer {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-sm);
		padding: var(--space-md);
		border-top: 1px solid rgba(255, 255, 255, 0.06);
		flex-shrink: 0;
	}

	.batch-btn {
		padding: 8px 14px;
		min-height: 36px;
		font-size: 0.8125rem;
		font-family: inherit;
		background: transparent;
		border: 1px solid rgba(255, 255, 255, 0.12);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: border-color var(--duration-fast), color var(--duration-fast), background var(--duration-fast);
	}

	.batch-btn:hover:not(:disabled) {
		border-color: rgba(255, 255, 255, 0.25);
		color: var(--color-text);
	}

	.batch-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.batch-btn-danger {
		background: var(--color-error);
		border-color: var(--color-error);
		color: #fff;
		font-weight: 600;
	}

	.batch-btn-danger:hover:not(:disabled) {
		background: color-mix(in srgb, var(--color-error) 85%, black);
		border-color: color-mix(in srgb, var(--color-error) 85%, black);
		color: #fff;
	}

	/* Mobile: bottom sheet */
	@media (max-width: 640px) {
		.batch-backdrop {
			padding: 0;
			align-items: flex-end;
		}

		.batch-dialog {
			width: 100%;
			position: fixed;
			inset: auto 0 0 0;
			max-height: 80vh;
			overflow-y: auto;
			border-radius: var(--radius-lg) var(--radius-lg) 0 0;
			border-bottom: none;
			padding-bottom: max(var(--space-md), env(safe-area-inset-bottom));
		}

		.grab-handle {
			display: block;
			width: 36px;
			height: 4px;
			margin: var(--space-sm) auto 0;
			border-radius: var(--radius-full);
			background: rgba(255, 255, 255, 0.2);
		}

		/* Remove sits at the thumb; DOM order is unchanged so Cancel keeps focus. */
		.batch-footer {
			flex-direction: column-reverse;
		}

		.batch-btn {
			width: 100%;
			min-height: 44px;
		}
	}
</style>
