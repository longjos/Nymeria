<script lang="ts">
	import { gpsStatus, gpsAgeMs, formatGpsAge } from '$lib/stores/gps';

	let status = $derived($gpsStatus);
	let fix = $derived(status.fix);

	let dotColor = $derived(
		!status.connected
			? 'var(--color-error)'
			: !fix || fix.mode <= 1
				? 'var(--color-warning)'
				: status.stale
					? 'var(--color-text-muted)'
					: fix.mode === 3
						? 'var(--color-success)'
						: 'var(--color-warning)'
	);

	let label = $derived(
		!status.connected ? 'GPS offline' : !fix || fix.mode <= 1 ? 'No fix' : fix.mode === 3 ? '3D' : '2D'
	);

	let ageSuffix = $derived.by(() => {
		if (!status.connected || !fix || fix.mode <= 1) return '';
		const ms = $gpsAgeMs;
		return ms == null ? '' : ` · ${formatGpsAge(ms)}`;
	});

	let title = $derived.by(() => {
		if (!status.connected) return status.error || 'GPS offline';
		const parts: string[] = [];
		if (status.type && status.target) parts.push(`${status.type} ${status.target}`);
		if (fix?.satellites) parts.push(`${fix.satellites} sats`);
		if (fix?.hdop) parts.push(`HDOP ${fix.hdop.toFixed(2)}`);
		if (fix?.accuracy) parts.push(`±${Math.round(fix.accuracy)} m`);
		return parts.join(' · ');
	});
</script>

{#if status.enabled}
	<div class="gps-status-pill" class:stale={status.stale} title={title || undefined}>
		<span class="gps-status-dot" style="background: {dotColor}"></span>
		<span class="gps-status-label">{label}</span>{ageSuffix}
	</div>
{/if}

<style>
	.gps-status-pill {
		position: absolute;
		top: 178px;
		left: calc(10px + var(--map-left-inset, 0px));
		z-index: var(--z-toolbar);
		height: 24px;
		display: inline-flex;
		align-items: center;
		gap: 5px;
		width: fit-content;
		padding: 0 var(--space-sm);
		background: rgba(22, 33, 62, 0.9);
		border-radius: var(--radius-full);
		font-size: 11px;
		color: var(--color-text-muted);
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
		white-space: nowrap;
		transition: opacity 0.15s;
	}

	.gps-status-pill.stale {
		opacity: 0.6;
	}

	.gps-status-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.gps-status-label {
		color: var(--color-text);
		font-weight: 600;
	}
</style>
