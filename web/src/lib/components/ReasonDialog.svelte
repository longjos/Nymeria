<script lang="ts">
	// One-field reason prompt, replacing the native window.prompt() the ride
	// boards used for cancel/release. Every one of those reasons is written to
	// the net timeline and the ICS-214 export, so the control that captures it
	// cannot be an unstyled, unvalidated, un-themeable browser modal that some
	// embedded webviews (including the Wails desktop shell) suppress outright.
	//
	// Same dialog contract as SweepPassedConfirm.svelte: backdrop click and
	// Escape cancel, aria-modal + data-blocks-escape so the global ride
	// shortcuts stay inert, focus moves in on mount, and the commit button
	// carries its own pending state and error line.
	let {
		title,
		label = 'Reason',
		confirmLabel = 'Confirm',
		required = true,
		danger = false,
		onConfirm,
		onCancel
	}: {
		title: string;
		label?: string;
		confirmLabel?: string;
		/** false = the backend accepts an empty reason (e.g. releasing a vehicle). */
		required?: boolean;
		danger?: boolean;
		onConfirm: (reason: string) => Promise<void>;
		onCancel: () => void;
	} = $props();

	let reason = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	let canSubmit = $derived(!submitting && (!required || reason.trim() !== ''));

	async function confirm() {
		if (!canSubmit) return;
		submitting = true;
		error = null;
		try {
			await onConfirm(reason.trim());
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save — try again.';
		} finally {
			submitting = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && !submitting) onCancel();
	}

	let el = $state<HTMLElement | null>(null);
	$effect(() => {
		el?.querySelector<HTMLElement>('textarea')?.focus();
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_click_events_have_key_events -->
<div class="rd-backdrop" role="presentation" onclick={() => !submitting && onCancel()}>
	<div
		class="rd"
		bind:this={el}
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-labelledby="rd-title"
		data-blocks-escape="true"
		onclick={(e) => e.stopPropagation()}
	>
		<h3 id="rd-title" class="rd-title">{title}</h3>
		<label class="rd-label" for="rd-reason">{label}{required ? '' : ' (optional)'}</label>
		<textarea
			id="rd-reason"
			class="rd-input"
			rows="3"
			bind:value={reason}
			disabled={submitting}
			aria-describedby={error ? 'rd-error' : undefined}
		></textarea>
		{#if error}<p id="rd-error" class="rd-error" role="alert">{error}</p>{/if}
		<div class="rd-actions">
			<button class="rd-btn rd-cancel" disabled={submitting} onclick={onCancel}>Cancel</button>
			<button
				class="rd-btn rd-confirm"
				class:danger
				disabled={!canSubmit}
				aria-busy={submitting}
				onclick={confirm}
			>
				{submitting ? 'Saving…' : confirmLabel}
			</button>
		</div>
	</div>
</div>

<style>
	.rd-backdrop {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		background: var(--color-scrim);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-md);
	}

	.rd {
		width: 100%;
		max-width: 360px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
	}

	.rd-title {
		font-size: 0.95rem;
		font-weight: 700;
	}

	.rd-label {
		font-size: 0.72rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
	}

	.rd-input {
		width: 100%;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
		font-size: 0.85rem;
		padding: var(--space-sm);
		resize: vertical;
	}

	.rd-input:disabled {
		opacity: 0.6;
	}

	.rd-error {
		margin: 0;
		font-size: 0.78rem;
		color: var(--color-error-text);
	}

	.rd-actions {
		display: flex;
		gap: var(--space-sm);
		justify-content: flex-end;
	}

	.rd-btn {
		min-height: 40px;
		padding: 0 var(--space-md);
		border-radius: var(--radius-sm);
		font-weight: 700;
		cursor: pointer;
	}

	.rd-cancel {
		background: none;
		border: 1px solid var(--color-primary);
		color: var(--color-text);
	}

	.rd-confirm {
		background: var(--color-accent);
		border: none;
		color: var(--color-on-accent);
	}

	.rd-confirm.danger {
		background: var(--color-error);
	}

	.rd-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
