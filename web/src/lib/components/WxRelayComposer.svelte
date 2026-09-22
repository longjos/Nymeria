<script lang="ts">
	import type { WxAlert } from '$lib/types';
	import { activeNet } from '$lib/stores/netcontrol';
	import { currentUser } from '$lib/stores/session';
	import { showToast } from '$lib/stores/toast';
	import { api } from '$lib/api';
	import { bulletinSummary } from '$lib/wxAlertMeta';

	let { alert, onClose }: { alert: WxAlert; onClose: () => void } = $props();

	const ncsCallsign = $activeNet?.ncsCallsign || $currentUser?.callsign || 'NCS';

	let note = $state(true);
	let bulletin = $state(false);
	let messages = $state(false);
	let text = $state(bulletinSummary(alert, ncsCallsign).slice(0, 67));
	let sending = $state(false);
	let error = $state<string | null>(null);

	let stationCount = $derived(alert.affects.stations.filter((s) => s.kind === 'station').length);
	let canSubmit = $derived((note || bulletin || messages) && !sending);

	let dialogEl = $state<HTMLElement | null>(null);

	// P0-3: the dialog declared aria-modal but never took focus, so Tab
	// continued through the page behind it and a screen reader never entered.
	$effect(() => {
		dialogEl?.querySelector<HTMLElement>('input, textarea, button')?.focus();
	});

	function trapTab(e: KeyboardEvent): void {
		if (e.key !== 'Tab' || !dialogEl) return;
		const focusable = Array.from(
			dialogEl.querySelectorAll<HTMLElement>('input, textarea, button:not([disabled])')
		).filter((el) => el.offsetParent !== null);
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

	async function submit(): Promise<void> {
		sending = true;
		error = null;
		try {
			const result = await api.relayWxAlert(alert.id, { note, bulletin, messages, text: text.slice(0, 67) });
			const parts: string[] = [];
			if (note) parts.push('note');
			if (bulletin && result.bulletinSent) parts.push('BLN0');
			if (messages) parts.push(`${result.messagesSent} messages`);
			if (parts.length === 0) {
				showToast('Nothing was sent — no relay method was selected.', 'error');
			} else {
				const pinned = note ? ' · pinned to the Situation Board' : '';
				showToast(`Relayed: ${parts.join(', ')}${pinned}`, 'success');
			}
			if (result.messagesFailed.length) {
				showToast(`Could not reach: ${result.messagesFailed.join(', ')}`, 'error');
			}
			onClose();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not relay this alert.';
		} finally {
			sending = false;
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="wx-relay-backdrop" role="presentation" onclick={onClose}>
	<div
		class="wx-relay"
		bind:this={dialogEl}
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-labelledby="wx-relay-title"
		data-blocks-escape="true"
		onclick={(e) => e.stopPropagation()}
	>
		<h2 id="wx-relay-title" class="wx-relay-title">Relay {alert.event}</h2>

		<label class="wx-toggle-row">
			<span class="wx-toggle-copy">
				<span class="wx-toggle-label">Pin as net note</span>
				<span class="wx-toggle-caption">Category weather · pinned to the Situation Board</span>
			</span>
			<input type="checkbox" bind:checked={note} />
		</label>

		<label class="wx-toggle-row">
			<span class="wx-toggle-copy">
				<span class="wx-toggle-label">Send APRS bulletin (BLN0)</span>
				<span class="wx-toggle-caption">Stations without internet will not see this alert otherwise.</span>
			</span>
			<input type="checkbox" bind:checked={bulletin} />
		</label>

		<label class="wx-toggle-row" class:wx-toggle-disabled={stationCount === 0}>
			<span class="wx-toggle-copy">
				<span class="wx-toggle-label">Message each roster station</span>
				<span class="wx-toggle-caption">
					{#if stationCount === 0}
						No roster stations are inside this alert.
					{:else}
						{stationCount} {stationCount === 1 ? 'station' : 'stations'} · sent one at a time via the message path
					{/if}
				</span>
			</span>
			<input type="checkbox" bind:checked={messages} disabled={stationCount === 0} />
		</label>

		{#if bulletin || messages}
			<div class="wx-relay-text-row">
				<textarea class="wx-relay-text" maxlength={67} bind:value={text} rows="2" aria-label="Bulletin / message text"></textarea>
				<!-- maxlength caps this at 67, so flagging 67 as an error flagged
				     valid text. Warn as it fills instead. -->
				<span class="wx-relay-counter" class:over={text.length >= 60}>{text.length}/67</span>
			</div>
		{/if}

		<div class="wx-relay-footer">
			{#if error}<p class="wx-relay-error" role="alert">{error}</p>{/if}
			<button class="wx-relay-btn" onclick={onClose} disabled={sending}>Cancel</button>
			<button class="wx-relay-btn wx-relay-btn-accent" onclick={submit} disabled={!canSubmit}>Relay</button>
		</div>
	</div>
</div>

<style>
	.wx-relay-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: flex-end;
		justify-content: center;
	}

	@media (min-width: 769px) {
		.wx-relay-backdrop {
			align-items: center;
		}
	}

	.wx-relay {
		width: 100%;
		max-width: 420px;
		max-height: 90vh;
		overflow-y: auto;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg) var(--radius-lg) 0 0;
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	@media (min-width: 769px) {
		.wx-relay {
			border-radius: var(--radius-lg);
		}
	}

	.wx-relay-title {
		font-size: 1rem;
		font-weight: 700;
	}

	.wx-toggle-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		min-height: 44px;
		cursor: pointer;
	}

	.wx-toggle-copy {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.wx-toggle-label {
		font-size: 0.85rem;
		font-weight: 600;
	}

	.wx-toggle-caption {
		font-size: 0.72rem;
		color: var(--color-text-muted);
	}

	.wx-toggle-row input[type='checkbox'] {
		flex-shrink: 0;
		width: 36px;
		height: 20px;
		appearance: none;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.15);
		border-radius: 10px;
		cursor: pointer;
		position: relative;
	}

	.wx-toggle-row input[type='checkbox']::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 14px;
		height: 14px;
		background: var(--color-text-muted);
		border-radius: 50%;
		transition: transform var(--duration-fast), background var(--duration-fast);
	}

	.wx-toggle-row input[type='checkbox']:checked {
		background: color-mix(in srgb, var(--color-success) 20%, transparent);
		border-color: color-mix(in srgb, var(--color-success) 40%, transparent);
	}

	.wx-toggle-row input[type='checkbox']:checked::after {
		transform: translateX(16px);
		background: var(--color-success);
	}

	.wx-relay-text-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.wx-relay-text {
		width: 100%;
		resize: vertical;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-family: monospace;
		font-size: 0.8rem;
		padding: var(--space-sm);
	}

	.wx-relay-counter {
		align-self: flex-end;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-relay-counter.over {
		color: var(--color-wx-warning-text);
	}

	.wx-relay-error {
		font-size: 0.75rem;
		color: var(--color-wx-warning-text);
	}

	.wx-relay-footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	/* Full-width so it sits directly above the button row it belongs to, where
	   the eye already is when Relay fails. */
	.wx-relay-error {
		flex: 1 1 100%;
	}

	.wx-toggle-disabled {
		opacity: 0.55;
	}

	.wx-relay-btn {
		min-height: 44px;
		padding: 0 var(--space-md);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-relay-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.wx-relay-btn-accent {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: white;
	}
</style>
