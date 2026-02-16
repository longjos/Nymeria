<script lang="ts">
	import { login, currentUser, isPending, isDenied, userStatus } from '$lib/stores/session';

	let name = $state('');
	let error = $state('');
	let loading = $state(false);

	// 3 states: 'form' | 'waiting' | 'denied'
	let viewState = $derived.by(() => {
		if ($isDenied) return 'denied';
		if ($isPending) return 'waiting';
		return 'form';
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!name.trim()) return;
		error = '';
		loading = true;
		try {
			await login(name.trim());
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
		} finally {
			loading = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') handleSubmit(e);
	}

	function tryAgain() {
		currentUser.set(null);
		name = '';
		error = '';
	}
</script>

<div class="login-overlay">
	<div class="login-card">
		<div class="logo-area">
			<img src="/nymeria-logo.png" alt="Nymeria" width="40" height="40" />
			<h1>Nymeria</h1>
		</div>

		{#if viewState === 'form'}
			<p class="subtitle">Enter your name to request access</p>

			<form onsubmit={handleSubmit}>
				<label class="field">
					<span>Display Name</span>
					<!-- svelte-ignore a11y_autofocus -->
					<input
						type="text"
						bind:value={name}
						placeholder="e.g. Alice, KD7BBC"
						maxlength="32"
						autofocus
						onkeydown={handleKeydown}
					/>
				</label>

				{#if error}
					<p class="error">{error}</p>
				{/if}

				<button type="submit" disabled={loading || !name.trim()}>
					{#if loading}
						Connecting...
					{:else}
						Request Access
					{/if}
				</button>
			</form>
		{:else if viewState === 'waiting'}
			<div class="waiting-state">
				<div class="spinner"></div>
				<p class="waiting-text">Waiting for admin approval...</p>
				<p class="waiting-hint">An administrator will review your request</p>
			</div>
		{:else if viewState === 'denied'}
			<div class="denied-state">
				<div class="denied-icon">&#10005;</div>
				<p class="denied-text">Access denied by administrator</p>
				<button class="try-again" onclick={tryAgain}>Try Again</button>
			</div>
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

	/* Waiting state */
	.waiting-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-md);
		padding: var(--space-xl) 0;
	}

	.spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--color-primary);
		border-top-color: var(--color-accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	.waiting-text {
		font-size: 0.95rem;
		color: var(--color-text);
	}

	.waiting-hint {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	/* Denied state */
	.denied-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-md);
		padding: var(--space-xl) 0;
	}

	.denied-icon {
		width: 48px;
		height: 48px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: color-mix(in srgb, var(--color-error) 15%, transparent);
		color: var(--color-error);
		border-radius: 50%;
		font-size: 1.5rem;
		font-weight: 700;
	}

	.denied-text {
		font-size: 0.95rem;
		color: var(--color-text);
	}

	.try-again {
		padding: 8px 20px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		color: var(--color-text);
		font-size: 0.85rem;
		cursor: pointer;
		transition: border-color var(--duration-fast);
	}

	.try-again:hover {
		border-color: var(--color-accent);
	}
</style>
