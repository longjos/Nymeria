<script lang="ts">
	import { pinRequired, login } from '$lib/stores/session';

	let name = $state('');
	let pin = $state('');
	let error = $state('');
	let loading = $state(false);
	let needsPin = $derived($pinRequired);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!name.trim()) return;
		error = '';
		loading = true;
		try {
			await login(name.trim(), pin || undefined);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
		} finally {
			loading = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') handleSubmit(e);
	}
</script>

<div class="login-overlay">
	<div class="login-card">
		<div class="logo-area">
			<img src="/nymeria-logo.png" alt="Nymeria" width="40" height="40" />
			<h1>Nymeria</h1>
		</div>

		<p class="subtitle">Choose a display name to connect</p>

		<form onsubmit={handleSubmit}>
			<label class="field">
				<span>Display Name</span>
				<input
					type="text"
					bind:value={name}
					placeholder="e.g. Alice, KD7BBC"
					maxlength="32"
					autofocus
					onkeydown={handleKeydown}
				/>
			</label>

			{#if needsPin}
				<label class="field">
					<span>Station PIN <small>(optional — for operator access)</small></span>
					<input
						type="password"
						bind:value={pin}
						placeholder="Leave blank for observer"
						maxlength="32"
						onkeydown={handleKeydown}
					/>
				</label>
			{/if}

			{#if error}
				<p class="error">{error}</p>
			{/if}

			<button type="submit" disabled={loading || !name.trim()}>
				{#if loading}
					Connecting...
				{:else}
					Connect
				{/if}
			</button>
		</form>

		{#if !needsPin}
			<p class="hint">No PIN configured — full access enabled</p>
		{/if}
	</div>
</div>

<style>
	.login-overlay {
		position: fixed;
		inset: 0;
		z-index: var(--z-overlay);
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--color-bg);
	}

	.login-card {
		width: 100%;
		max-width: 360px;
		padding: var(--space-xl);
		margin: var(--space-md);
	}

	.logo-area {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-bottom: var(--space-lg);
		color: var(--color-accent);
	}

	.logo-area h1 {
		font-size: 1.5rem;
		font-weight: 700;
		letter-spacing: 0.05em;
	}

	.subtitle {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		margin-bottom: var(--space-lg);
	}

	form {
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.field span {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.field small {
		opacity: 0.7;
	}

	.field input {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text);
		padding: 10px 12px;
		font-size: 0.9rem;
		outline: none;
		transition: border-color var(--duration-fast);
	}

	.field input:focus {
		border-color: var(--color-accent);
	}

	.field input::placeholder {
		color: var(--color-text-muted);
		opacity: 0.5;
	}

	.error {
		font-size: 0.8rem;
		color: var(--color-error);
	}

	button[type="submit"] {
		padding: 10px;
		background: var(--color-accent);
		border: none;
		border-radius: var(--radius-md);
		color: white;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
		transition: opacity var(--duration-fast);
	}

	button[type="submit"]:hover:not(:disabled) {
		opacity: 0.9;
	}

	button[type="submit"]:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.hint {
		margin-top: var(--space-md);
		font-size: 0.75rem;
		color: var(--color-text-muted);
		text-align: center;
		opacity: 0.6;
	}
</style>
