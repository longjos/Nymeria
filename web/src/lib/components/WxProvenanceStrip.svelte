<script lang="ts">
	import { wxLinkStatus, wxClock } from '$lib/stores/wxAlerts';
	import { canOperate } from '$lib/stores/session';
	import { openSettings } from '$lib/stores/ui';
	import { showToast } from '$lib/stores/toast';
	import { linkStateText, clockWithSeconds } from '$lib/wxAlertTime';
	import { api } from '$lib/api';

	let open = $state(false);

	let sentence = $derived(linkStateText($wxLinkStatus, $wxClock));
	let dotClass = $derived(
		$wxLinkStatus.state === 'live'
			? 'wx-prov-dot-live'
			: $wxLinkStatus.state === 'stale'
				? 'wx-prov-dot-stale'
				: $wxLinkStatus.state === 'down'
					? 'wx-prov-dot-down'
					: 'wx-prov-dot-off'
	);

	function toggle(): void {
		open = !open;
	}

	async function retryNow(): Promise<void> {
		try {
			await api.wxRefresh();
			showToast('Checking NWS…', 'info');
		} catch {
			showToast('Could not reach the server to retry.', 'error');
		}
	}
</script>

<div class="wx-provenance">
	<button class="wx-prov-btn" onclick={toggle} aria-expanded={open} aria-live="off">
		<span class="wx-prov-dot {dotClass}" aria-hidden="true"></span>
		<span class="wx-prov-sentence">{sentence}</span>
		<span class="wx-prov-caret" class:open aria-hidden="true">▾</span>
	</button>

	{#if open}
		<div class="wx-prov-pop" role="group" aria-label="NWS link details">
			<p>Last attempt {clockWithSeconds($wxLinkStatus.lastAttemptAt)}</p>
			{#if $wxLinkStatus.lastError}<p>Last error: {$wxLinkStatus.lastError}</p>{/if}
			<p>{$wxLinkStatus.regionCount} alerts in region · {$wxLinkStatus.inAreaCount} in watch area · {$wxLinkStatus.nearbyCount} nearby</p>
			<p>Zones cached {$wxLinkStatus.zonesCached} · missing {$wxLinkStatus.zonesMissing}</p>
			{#if $canOperate}
				<button class="wx-prov-row" onclick={retryNow}>Retry now</button>
			{/if}
			{#if !$wxLinkStatus.contactConfigured}
				<p class="wx-prov-nag">
					NWS requires a contact e-mail in the User-Agent and returns 403 without one. Add it in
					<button class="wx-prov-link" onclick={() => openSettings('wxalerts')}>Settings →</button>
				</p>
			{/if}
		</div>
	{/if}
</div>

<style>
	.wx-provenance {
		position: relative;
	}

	.wx-prov-btn {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		min-height: 36px;
		padding: 0 var(--space-md);
		background: var(--color-bg);
		border: none;
		border-bottom: 1px solid var(--color-primary);
		color: var(--color-text);
		font-size: 0.75rem;
		text-align: left;
		cursor: pointer;
	}

	.wx-prov-dot {
		flex-shrink: 0;
		width: 6px;
		height: 6px;
		border-radius: 50%;
	}

	.wx-prov-dot-live { background: var(--color-success); }
	.wx-prov-dot-off { background: var(--color-text-muted); }
	.wx-prov-dot-down { background: var(--color-wx-warning); }
	.wx-prov-dot-stale {
		background: var(--color-wx-watch);
		animation: wx-prov-pulse 1.6s ease-in-out infinite;
	}

	@media (prefers-reduced-motion: reduce) {
		.wx-prov-dot-stale {
			animation: none;
			background: transparent;
			border: 1.5px solid var(--color-wx-watch);
		}
	}

	@keyframes wx-prov-pulse {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.35; }
	}

	.wx-prov-sentence {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-muted);
	}

	.wx-prov-caret {
		flex-shrink: 0;
		color: var(--color-text-muted);
		transition: transform var(--duration-fast);
	}

	.wx-prov-caret.open {
		transform: rotate(180deg);
	}

	.wx-prov-pop {
		position: absolute;
		top: 100%;
		left: var(--space-sm);
		right: var(--space-sm);
		z-index: var(--z-overlay);
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		padding: var(--space-sm) var(--space-md);
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 0.75rem;
		color: var(--color-text-muted);
	}

	.wx-prov-row {
		align-self: flex-start;
		min-height: 32px;
		padding: 0 var(--space-sm);
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.75rem;
		font-weight: 600;
		cursor: pointer;
	}

	.wx-prov-row:hover,
	.wx-prov-row:focus-visible {
		background: var(--color-accent);
		color: white;
	}

	.wx-prov-nag {
		color: var(--color-wx-watch);
	}

	.wx-prov-link {
		background: none;
		border: none;
		padding: 0;
		color: var(--color-accent);
		font-size: inherit;
		font-weight: 600;
		cursor: pointer;
		text-decoration: underline;
	}
</style>
