<script lang="ts">
	import { packets, filteredPackets, inspectorPaused, packetTypeFilter, callsignFilter, sourceFilter, totalPacketCount, clearPackets } from '$lib/stores/packets';
	import type { APRSPacketType, RawPacket } from '$lib/types';
	import PacketDetail from './PacketDetail.svelte';

	let expandedId = $state<string | null>(null);
	let listEl = $state<HTMLDivElement>();
	let userScrolledUp = $state(false);

	const packetTypes: APRSPacketType[] = ['position', 'message', 'object', 'item', 'weather', 'status', 'telemetry', 'micE', 'query', 'thirdParty', 'unknown'];

	function typeColor(t: string): string {
		switch (t) {
			case 'position': return 'position';
			case 'message': return 'message';
			case 'weather': return 'weather';
			case 'telemetry': return 'telemetry';
			case 'object': case 'item': return 'object';
			case 'micE': return 'micE';
			default: return 'muted';
		}
	}

	function formatTime(ts: string): string {
		const d = new Date(ts);
		return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
	}

	function formatAddress(addr: { call: string; ssid?: number }): string {
		if (addr.ssid) return `${addr.call}-${addr.ssid}`;
		return addr.call;
	}

	function truncateRaw(raw: string): string {
		if (raw.length > 80) return raw.slice(0, 80) + '...';
		return raw;
	}

	function pktKey(pkt: RawPacket): string {
		return pkt.timestamp + pkt.raw;
	}

	function toggleExpand(key: string) {
		expandedId = expandedId === key ? null : key;
	}

	function handleScroll() {
		if (!listEl) return;
		userScrolledUp = listEl.scrollTop > 50;
	}

	function jumpToLatest() {
		if (listEl) {
			listEl.scrollTop = 0;
			userScrolledUp = false;
		}
	}

	// Auto-scroll to top when new packets arrive (newest-first list)
	$effect(() => {
		// Access the reactive value to track changes
		$filteredPackets;
		if (!userScrolledUp && listEl) {
			listEl.scrollTop = 0;
		}
	});

	// Collect unique sources for filter dropdown
	let sources = $derived.by(() => {
		const s = new Set<string>();
		for (const p of $packets) {
			if (p.source) s.add(p.source);
		}
		return Array.from(s).sort();
	});
</script>

<div class="inspector-panel">
	<!-- Header -->
	<div class="inspector-header">
		<div class="header-left">
			<svg class="header-icon" width="18" height="18" viewBox="0 0 16 16" fill="none">
				<rect x="1" y="3" width="14" height="11" rx="1" stroke="currentColor" stroke-width="1.3"/>
				<path d="M3 6h2M3 8.5h4M3 11h3" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
				<path d="M1 5.5h14" stroke="currentColor" stroke-width="1.3"/>
			</svg>
			<span class="header-title">Packet Inspector</span>
			<span class="header-count">{$totalPacketCount}</span>
		</div>
		<div class="header-right">
			<button
				class="ctrl-btn"
				class:paused={$inspectorPaused}
				onclick={() => inspectorPaused.update((v) => !v)}
				title={$inspectorPaused ? 'Resume' : 'Pause'}
			>
				{#if $inspectorPaused}
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><path d="M5 3l8 5-8 5V3z" fill="currentColor"/></svg>
				{:else}
					<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><rect x="3" y="3" width="3.5" height="10" rx="0.5" fill="currentColor"/><rect x="9.5" y="3" width="3.5" height="10" rx="0.5" fill="currentColor"/></svg>
				{/if}
			</button>
			<button class="ctrl-btn" onclick={clearPackets} title="Clear">
				<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><path d="M3 3l10 10M13 3L3 13" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
			</button>
		</div>
	</div>

	<!-- Filter bar -->
	<div class="filter-bar">
		<select bind:value={$packetTypeFilter} class="filter-select">
			<option value="">All types</option>
			{#each packetTypes as t}
				<option value={t}>{t}</option>
			{/each}
		</select>
		<input
			type="text"
			class="filter-input"
			placeholder="Callsign..."
			bind:value={$callsignFilter}
		/>
		<select bind:value={$sourceFilter} class="filter-select source-select">
			<option value="">All sources</option>
			{#each sources as s}
				<option value={s}>{s}</option>
			{/each}
		</select>
	</div>

	<!-- Packet list -->
	<div class="packet-list" bind:this={listEl} onscroll={handleScroll}>
		{#if $filteredPackets.length === 0}
			<div class="empty-state">
				{#if $totalPacketCount === 0}
					<p>Waiting for packets...</p>
				{:else if $inspectorPaused}
					<p>Inspector paused. {$totalPacketCount} total packets received.</p>
				{:else}
					<p>No packets match filters.</p>
				{/if}
			</div>
		{:else}
			{#each $filteredPackets as pkt (pkt.timestamp + pkt.raw)}
				{@const key = pktKey(pkt)}
				<div class="packet-row" class:expanded={expandedId === key}>
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div class="packet-summary" onclick={() => toggleExpand(key)}>
						<span class="pkt-time">{formatTime(pkt.timestamp)}</span>
						<span class="pkt-type {typeColor(pkt.packetType)}">{pkt.packetType}</span>
						<span class="pkt-source">{pkt.source}</span>
						<span class="pkt-from">{formatAddress(pkt.from)}</span>
						<span class="pkt-arrow">&rarr;</span>
						<span class="pkt-to">{formatAddress(pkt.to)}</span>
						<span class="pkt-raw">{truncateRaw(pkt.raw)}</span>
					</div>
					{#if expandedId === key}
						<PacketDetail packet={pkt} />
					{/if}
				</div>
			{/each}
		{/if}
	</div>

	<!-- Jump to latest -->
	{#if userScrolledUp && $filteredPackets.length > 0}
		<button class="jump-btn" onclick={jumpToLatest}>
			<svg width="12" height="12" viewBox="0 0 16 16" fill="none"><path d="M8 12V4M4 8l4-4 4 4" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
			Jump to latest
		</button>
	{/if}
</div>

<style>
	.inspector-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
	}

	/* Header */
	.inspector-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 10px 12px;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.header-left {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.header-icon {
		color: var(--color-accent);
		flex-shrink: 0;
	}

	.header-title {
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.header-count {
		font-size: 0.65rem;
		font-weight: 700;
		padding: 1px 6px;
		border-radius: 10px;
		background: var(--color-primary);
		color: var(--color-text-muted);
		font-variant-numeric: tabular-nums;
	}

	.header-right {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.ctrl-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: none;
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		transition: color 0.15s, background 0.15s;
	}

	.ctrl-btn:hover {
		color: var(--color-text);
		background: var(--color-primary);
	}

	.ctrl-btn.paused {
		color: #22c55e;
		border-color: #22c55e;
	}

	/* Filter bar */
	.filter-bar {
		display: flex;
		gap: 6px;
		padding: 6px 12px;
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.filter-select {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.72rem;
		padding: 4px 6px;
		min-width: 0;
	}

	.filter-input {
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.72rem;
		padding: 4px 8px;
		flex: 1;
		min-width: 0;
	}

	.filter-input::placeholder {
		color: var(--color-text-muted);
	}

	.source-select {
		max-width: 120px;
	}

	/* Packet list */
	.packet-list {
		flex: 1;
		overflow-y: auto;
		min-height: 0;
	}

	.empty-state {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 32px 16px;
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}

	.packet-row {
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
	}

	.packet-row.expanded {
		background: rgba(255, 255, 255, 0.02);
	}

	.packet-summary {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 5px 12px;
		cursor: pointer;
		transition: background 0.1s;
		overflow: hidden;
	}

	.packet-summary:hover {
		background: var(--color-primary);
	}

	.pkt-time {
		font-size: 0.68rem;
		color: var(--color-text-muted);
		font-family: 'JetBrains Mono', 'Fira Code', monospace;
		flex-shrink: 0;
	}

	.pkt-type {
		display: inline-block;
		padding: 1px 5px;
		border-radius: 6px;
		font-size: 0.6rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		flex-shrink: 0;
		min-width: 50px;
		text-align: center;
	}

	.pkt-type.position { background: rgba(var(--accent-rgb, 0, 191, 255), 0.15); color: var(--color-accent); }
	.pkt-type.message { background: rgba(96, 165, 250, 0.15); color: #60a5fa; }
	.pkt-type.weather { background: rgba(245, 158, 11, 0.15); color: #f59e0b; }
	.pkt-type.telemetry { background: rgba(168, 85, 247, 0.15); color: #a855f7; }
	.pkt-type.object { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
	.pkt-type.micE { background: rgba(236, 72, 153, 0.15); color: #ec4899; }
	.pkt-type.muted { background: rgba(148, 163, 184, 0.12); color: #94a3b8; }

	.pkt-source {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		background: var(--color-primary);
		padding: 1px 4px;
		border-radius: 4px;
		flex-shrink: 0;
		max-width: 70px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.pkt-from, .pkt-to {
		font-size: 0.72rem;
		font-family: 'JetBrains Mono', 'Fira Code', monospace;
		font-weight: 600;
		color: var(--color-text);
		flex-shrink: 0;
	}

	.pkt-arrow {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.pkt-raw {
		font-size: 0.65rem;
		font-family: 'JetBrains Mono', 'Fira Code', monospace;
		color: var(--color-text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}

	/* Jump to latest */
	.jump-btn {
		position: absolute;
		top: 100px;
		left: 50%;
		transform: translateX(-50%);
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 5px 12px;
		background: var(--color-surface);
		border: 1px solid var(--color-accent);
		border-radius: 16px;
		color: var(--color-accent);
		font-size: 0.72rem;
		font-weight: 600;
		cursor: pointer;
		box-shadow: var(--shadow-md);
		z-index: 2;
	}

	.jump-btn:hover {
		background: var(--color-primary);
	}
</style>
