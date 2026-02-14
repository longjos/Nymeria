<script lang="ts">
	import { onMount } from 'svelte';
	import { pendingRequests, loadPendingRequests } from '$lib/stores/session';
	import { api } from '$lib/api';
	import type { Role } from '$lib/types';

	let selectedRoles = $state<Record<string, Role>>({});
	let processing = $state<Record<string, boolean>>({});

	onMount(() => {
		loadPendingRequests();
	});

	function getRole(userId: string): Role {
		return selectedRoles[userId] ?? 'observer';
	}

	function setRole(userId: string, role: Role) {
		selectedRoles = { ...selectedRoles, [userId]: role };
	}

	async function approve(userId: string) {
		processing = { ...processing, [userId]: true };
		try {
			await api.approveUser(userId, getRole(userId));
		} catch (err) {
			console.error('Failed to approve user:', err);
		} finally {
			processing = { ...processing, [userId]: false };
		}
	}

	async function deny(userId: string) {
		processing = { ...processing, [userId]: true };
		try {
			await api.denyUser(userId);
		} catch (err) {
			console.error('Failed to deny user:', err);
		} finally {
			processing = { ...processing, [userId]: false };
		}
	}

	function timeSince(dateStr: string): string {
		const secs = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
		if (secs < 60) return 'just now';
		const mins = Math.floor(secs / 60);
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		return `${hrs}h ago`;
	}
</script>

{#if $pendingRequests.length > 0}
	<div class="pending-section">
		<h3 class="section-title">
			Access Requests
			<span class="badge">{$pendingRequests.length}</span>
		</h3>

		<div class="request-list">
			{#each $pendingRequests as req (req.id)}
				<div class="request-item">
					<div class="request-info">
						<span class="request-name">{req.name}</span>
						<span class="request-time">{timeSince(req.connectedAt)}</span>
					</div>

					<div class="request-actions">
						<select
							value={getRole(req.id)}
							onchange={(e) => setRole(req.id, (e.target as HTMLSelectElement).value as Role)}
							disabled={processing[req.id]}
						>
							<option value="observer">Observer</option>
							<option value="plotter">Plotter</option>
							<option value="operator">Operator</option>
						</select>

						<button
							class="btn-approve"
							onclick={() => approve(req.id)}
							disabled={processing[req.id]}
						>
							Approve
						</button>

						<button
							class="btn-deny"
							onclick={() => deny(req.id)}
							disabled={processing[req.id]}
						>
							Deny
						</button>
					</div>
				</div>
			{/each}
		</div>
	</div>
{/if}

<style>
	.pending-section {
		margin-bottom: var(--space-lg);
	}

	.section-title {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text);
		margin-bottom: var(--space-sm);
		display: flex;
		align-items: center;
		gap: var(--space-xs);
	}

	.badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
		background: var(--color-accent);
		color: white;
		border-radius: 9px;
		font-size: 0.7rem;
		font-weight: 700;
	}

	.request-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.request-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
	}

	.request-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.request-name {
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--color-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.request-time {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.request-actions {
		display: flex;
		align-items: center;
		gap: var(--space-xs);
		flex-shrink: 0;
	}

	.request-actions select {
		padding: 4px 6px;
		font-size: 0.75rem;
		background: var(--color-bg);
		color: var(--color-text);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	.btn-approve {
		padding: 4px 10px;
		font-size: 0.75rem;
		font-weight: 600;
		background: var(--color-success, #28a745);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		cursor: pointer;
		transition: opacity var(--duration-fast);
	}

	.btn-approve:hover:not(:disabled) {
		opacity: 0.85;
	}

	.btn-approve:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-deny {
		padding: 4px 10px;
		font-size: 0.75rem;
		font-weight: 600;
		background: transparent;
		color: var(--color-error);
		border: 1px solid var(--color-error);
		border-radius: var(--radius-sm);
		cursor: pointer;
		transition: all var(--duration-fast);
	}

	.btn-deny:hover:not(:disabled) {
		background: color-mix(in srgb, var(--color-error) 10%, transparent);
	}

	.btn-deny:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
