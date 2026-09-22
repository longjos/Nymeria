<script lang="ts">
	// Firing a shutoff reroutes every rider behind it and is meant to feel
	// consequential — a real confirm, not a bare button. Chrome copies
	// PhaseChangeDialog.svelte's backdrop/focus-trap pattern exactly (the
	// established precedent in this codebase for a ride-mode confirm dialog).
	import type { ShutoffPoint } from '$lib/types';
	import { fireShutoff } from '$lib/stores/course';
	import { clock } from '$lib/wxAlertTime';
	import { showToast, announce } from '$lib/stores/toast';

	let { shutoff, onClose }: { shutoff: ShutoffPoint; onClose: (fired: boolean) => void } = $props();

	let note = $state('');
	let rerouteCount = $state('');
	let bibText = $state('');
	let bibWithheld = $state(false);
	let pending = $state(false);
	let error = $state<string | null>(null);

	let early = $derived.by(() => {
		const ms = Date.parse(shutoff.scheduledAt) - Date.now();
		if (ms <= 0) return null;
		const mins = Math.round(ms / 60000);
		return mins > 0 ? `${mins} min early` : null;
	});

	async function fire(): Promise<void> {
		pending = true;
		error = null;
		const bibs = bibText.split(/[\s,]+/).map((b) => b.trim()).filter(Boolean);
		try {
			const result = await fireShutoff(shutoff.id, {
				note: note.trim(),
				bibs,
				rerouteCount: Number(rerouteCount) || 0,
				bibWithheld
			});
			announce(`${shutoff.name} shutoff fired`);
			showToast(`${shutoff.name} fired — ${bibs.length} bib${bibs.length === 1 ? '' : 's'}${result.shutoff.rerouteCount ? `, +${result.shutoff.rerouteCount} rerouted` : ''}`, 'info', 6000);
			onClose(true);
		} catch (e: any) {
			if (e?.body?.code === 'not_planned') {
				error = 'Already fired or cancelled by someone else.';
			} else {
				error = e?.message ?? 'Could not fire this shutoff.';
			}
		} finally {
			pending = false;
		}
	}

	let dialogEl = $state<HTMLElement | null>(null);
	let cancelEl = $state<HTMLButtonElement | null>(null);
	$effect(() => {
		cancelEl?.focus();
	});

	function trapTab(e: KeyboardEvent): void {
		if (e.key !== 'Tab' || !dialogEl) return;
		const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>('input, select, textarea, button:not([disabled])'));
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
		if (e.key === 'Escape') {
			if (!pending) onClose(false);
		} else {
			trapTab(e);
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="fd-backdrop" role="presentation" onclick={() => !pending && onClose(false)}>
	<div
		class="fd"
		bind:this={dialogEl}
		role="alertdialog"
		tabindex="-1"
		aria-modal="true"
		aria-labelledby="fd-title"
		aria-describedby="fd-stakes"
		data-blocks-escape="true"
		onclick={(e) => e.stopPropagation()}
	>
		<h2 id="fd-title" class="fd-title">Fire shutoff</h2>
		<div class="fd-identity">
			<span class="fd-name">&#9647; {shutoff.name}</span>
			<span class="fd-sub">Scheduled {clock(shutoff.scheduledAt)}{early ? ` · ${early}` : ''}</span>
		</div>

		<blockquote class="fd-readback" aria-label="On-air instructions">
			{shutoff.rerouteInstructions || `Riders not past ${shutoff.name}: turn ${shutoff.rerouteDirection || '—'} onto ${shutoff.rerouteDestination || '—'}.`}
		</blockquote>

		<ul class="fd-stakes" id="fd-stakes">
			<li class="fd-row"><span class="fd-icon" aria-hidden="true">!</span><span>Every rider behind this point is directed {shutoff.rerouteDirection || 'off-course'} onto {shutoff.rerouteDestination || 'the reroute'} and leaves the supported count.</span></li>
			<li class="fd-row"><span class="fd-icon" aria-hidden="true">&#9638;</span><span>Logged to the timeline and shown to every operator. Un-firing needs net control and a reason.</span></li>
		</ul>

		<div class="fd-form-row">
			<div class="fd-form-group">
				<label for="fd-count">Riders rerouted (no bib)</label>
				<input id="fd-count" type="number" min="0" inputmode="numeric" bind:value={rerouteCount} />
			</div>
			<div class="fd-form-group">
				<label for="fd-bibs">Bibs rerouted</label>
				<input id="fd-bibs" type="text" autocomplete="off" placeholder="112 340 77" bind:value={bibText} />
			</div>
		</div>
		<label class="fd-warn-check"><input type="checkbox" bind:checked={bibWithheld} /> Withhold bibs from the timeline</label>
		<div class="fd-form-group">
			<label for="fd-note">Note</label>
			<input id="fd-note" type="text" bind:value={note} />
		</div>

		{#if error}<div class="fd-error" role="alert">{error}</div>{/if}

		<div class="fd-actions">
			<button class="fd-btn" bind:this={cancelEl} onclick={() => onClose(false)} disabled={pending}>Keep open</button>
			<button class="fd-btn fd-btn-danger" onclick={fire} disabled={pending} aria-busy={pending}>{pending ? 'Firing…' : `Fire "${shutoff.name}"`}</button>
		</div>
	</div>
</div>

<style>
	.fd-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.fd {
		width: 100%;
		max-width: 440px;
		max-height: 90vh;
		overflow-y: auto;
		background: var(--color-surface);
		border: 1px solid var(--color-error);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.fd-title {
		font-size: 1.05rem;
		font-weight: 700;
	}

	.fd-identity {
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
	}

	.fd-name {
		font-weight: 700;
		font-size: 0.95rem;
	}

	.fd-sub {
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	.fd-readback {
		margin: 0;
		border-left: 3px solid var(--color-accent);
		padding: var(--space-sm);
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
		background: var(--color-bg);
	}

	.fd-stakes {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.fd-row {
		display: flex;
		gap: var(--space-sm);
		font-size: 0.8rem;
		align-items: flex-start;
	}

	.fd-icon {
		flex-shrink: 0;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--color-raised-strong);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.7rem;
		font-weight: 700;
	}

	.fd-form-row {
		display: flex;
		gap: var(--space-sm);
	}

	.fd-form-group {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
		min-width: 0;
	}

	.fd-form-group label {
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.fd-form-group input {
		min-height: 40px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		padding: 0 var(--space-sm);
	}

	.fd-warn-check {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}

	.fd-error {
		color: var(--color-error-text);
		font-size: 0.8rem;
	}

	.fd-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
		margin-top: var(--space-xs);
	}

	.fd-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.fd-btn-danger {
		background: var(--color-error);
		border: none;
		color: var(--color-on-accent);
	}

	.fd-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
