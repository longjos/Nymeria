<script lang="ts">
	import { currentUser, logout } from '$lib/stores/session';

	let open = $state(false);

	function handleLogout() {
		open = false;
		logout();
	}

	const roleLabels: Record<string, string> = {
		observer: 'Observer',
		plotter: 'Plotter',
		operator: 'Operator',
		admin: 'Admin'
	};
</script>

{#if $currentUser}
	<div class="user-menu">
		<button
			class="user-btn"
			onclick={() => (open = !open)}
			onblur={() => setTimeout(() => (open = false), 200)}
			title={$currentUser.name}
		>
			<span class="user-initial">{$currentUser.name[0].toUpperCase()}</span>
		</button>

		{#if open}
			<div class="dropdown">
				<div class="user-info">
					<span class="user-name">{$currentUser.name}</span>
					<span class="user-role">{roleLabels[$currentUser.role] ?? $currentUser.role}</span>
				</div>
				<button class="dropdown-btn" onmousedown={handleLogout}>
					Disconnect
				</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.user-menu {
		position: relative;
	}

	.user-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 30px;
		height: 30px;
		border-radius: 50%;
		background: var(--color-accent);
		border: none;
		color: white;
		font-size: 0.75rem;
		font-weight: 700;
		cursor: pointer;
		transition: opacity var(--duration-fast);
	}

	.user-btn:hover {
		opacity: 0.85;
	}

	.dropdown {
		position: absolute;
		top: calc(100% + 6px);
		right: 0;
		min-width: 160px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-md);
		overflow: hidden;
	}

	.user-info {
		padding: 10px 12px;
		border-bottom: 1px solid var(--color-primary);
	}

	.user-name {
		display: block;
		font-size: 0.85rem;
		font-weight: 600;
	}

	.user-role {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.dropdown-btn {
		display: block;
		width: 100%;
		padding: 8px 12px;
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		text-align: left;
		cursor: pointer;
		transition: background var(--duration-fast), color var(--duration-fast);
	}

	.dropdown-btn:hover {
		background: var(--color-primary);
		color: var(--color-text);
	}
</style>
