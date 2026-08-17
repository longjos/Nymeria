<script lang="ts">
	import type { KissTncInfo, TransportStatus } from '$lib/types';
	import {
		mergeTncOptions,
		persistTncValue,
		parseTncValue,
		tncKey,
		MANUAL_TNC_VALUE
	} from '$lib/kissTncs';
	import { kissLinkState, kissLinkLabel, kissLinkHint } from '$lib/serialPorts';

	let {
		idPrefix,
		host = $bindable('localhost'),
		port = $bindable(8001),
		tncs = [],
		tncsLoading = false,
		tncsError = null,
		onrefresh,
		liveStatus = null
	}: {
		idPrefix: string;
		host: string | undefined;
		port: number | undefined;
		tncs: KissTncInfo[];
		tncsLoading: boolean;
		tncsError: string | null;
		onrefresh: () => void;
		liveStatus?: TransportStatus | null;
	} = $props();

	let manualMode = $state(false);

	let merged = $derived(mergeTncOptions(tncs, host, port));
	let currentKey = $derived(host?.trim() ? tncKey(host, port) : '');

	let selectedOption = $derived.by(() => {
		if (manualMode) return MANUAL_TNC_VALUE;
		if (!currentKey) return '';
		const hit = merged.find((t) => persistTncValue(t) === currentKey);
		return hit ? persistTncValue(hit) : MANUAL_TNC_VALUE;
	});

	let oneHighlight = $derived.by(() => {
		const hits = merged.filter((t) => t.highlight && t.present !== false);
		return hits.length === 1 ? hits[0] : null;
	});

	let rematch = $derived.by(() => {
		if (!currentKey || !oneHighlight) return null;
		const missing = merged.some((t) => t.present === false && persistTncValue(t) === currentKey);
		return missing ? oneHighlight : null;
	});

	function onTncChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (value === MANUAL_TNC_VALUE) {
			manualMode = true;
			return;
		}
		manualMode = false;
		const parsed = parseTncValue(value);
		if (parsed) {
			host = parsed.host;
			port = parsed.port;
		}
	}

	function useTnc(t: KissTncInfo) {
		manualMode = false;
		host = t.host;
		port = t.port;
	}
</script>

<div class="kiss-fields">
	<div class="field-row port-row">
		<label for="{idPrefix}-tnc">KISS TCP TNC</label>
		<button type="button" class="refresh-btn" onclick={onrefresh} aria-label="Refresh KISS TCP TNCs" disabled={tncsLoading}>
			{tncsLoading ? '…' : 'Refresh'}
		</button>
		<select id="{idPrefix}-tnc" value={selectedOption} onchange={onTncChange}>
			<option value="">Select a TNC…</option>
			{#each merged as t}
				<option value={persistTncValue(t)}>
					{t.highlight && t.present !== false ? `${t.label} — suggested` : t.label}
				</option>
			{/each}
			<option value={MANUAL_TNC_VALUE}>Enter host and port manually…</option>
		</select>
	</div>

	{#if selectedOption === MANUAL_TNC_VALUE || manualMode}
		<div class="field-group">
			<div class="field-row half">
				<label for="{idPrefix}-host">Host</label>
				<input
					id="{idPrefix}-host"
					type="text"
					bind:value={host}
					placeholder="localhost"
					spellcheck="false"
					autocomplete="off"
					autocapitalize="off"
				/>
			</div>
			<div class="field-row half">
				<label for="{idPrefix}-port">Port</label>
				<input id="{idPrefix}-port" type="number" bind:value={port} placeholder="8001" />
			</div>
		</div>
	{/if}

	{#if rematch}
		<div class="banner">
			<span>{currentKey} is not present. Use {rematch.label}?</span>
			<button type="button" class="banner-btn" onclick={() => useTnc(rematch)}>Use this TNC</button>
		</div>
	{:else if oneHighlight && (!host?.trim() || merged.some((t) => t.present === false && persistTncValue(t) === currentKey))}
		<div class="banner">
			<span>Direwolf / KISS TCP found: {oneHighlight.label}.</span>
			<button type="button" class="banner-btn" onclick={() => useTnc(oneHighlight)}>Use this TNC</button>
		</div>
	{/if}

	{#if tncs.length === 0 && !tncsLoading}
		<p class="field-help">
			Looking for <code>_kiss-tnc._tcp</code> (Direwolf 1.7+ on Linux/macOS) and
			<code>localhost:8001</code>. Windows Direwolf does not advertise mDNS — if it is on this
			machine, start it and Refresh, or enter host/port manually.
		</p>
	{/if}

	{#if tncsError}
		<p class="field-help warn">{tncsError}</p>
	{/if}

	<p class="field-help">
		Direwolf default is port 8001 (KISS), not 8000 (AGWPE). Saved as host + port.
	</p>

	{#if liveStatus}
		{@const link = kissLinkState(liveStatus)}
		<div class="link-status">
			<span class="chip port" class:on={liveStatus.connected} title="TCP connected to the KISS port.">
				{liveStatus.connected ? 'TCP up' : 'TCP down'}
			</span>
			<span class="chip {link}" title={kissLinkHint(link)}>{kissLinkLabel(link)}</span>
			<span class="counts">RX {liveStatus.packetsRx ?? 0} · TX {liveStatus.packetsTx ?? 0}</span>
		</div>
	{/if}
</div>

<style>
	.port-row {
		position: relative;
	}

	.refresh-btn {
		position: absolute;
		top: 0;
		right: 0;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.72rem;
		font-weight: 600;
		cursor: pointer;
		padding: 0;
	}

	.refresh-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.field-row {
		margin-bottom: var(--space-sm);
	}

	.field-row label {
		display: block;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		margin-bottom: 2px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.field-row input[type='text'],
	.field-row input[type='number'],
	.field-row select {
		width: 100%;
		padding: 6px 10px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.82rem;
		font-family: inherit;
		outline: none;
	}

	.field-row select {
		appearance: none;
		cursor: pointer;
	}

	.field-group {
		display: flex;
		gap: var(--space-sm);
	}

	.field-row.half {
		flex: 1;
	}

	.field-help {
		margin: 0 0 var(--space-sm);
		font-size: 0.72rem;
		color: var(--color-text-muted);
		line-height: 1.4;
	}

	.field-help.warn {
		color: #fbbf24;
	}

	.field-help code {
		font-size: 0.7rem;
	}

	.banner {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs);
		margin-bottom: var(--space-sm);
		padding: 6px 8px;
		border-radius: var(--radius-sm);
		background: rgba(56, 189, 248, 0.08);
		border: 1px solid rgba(56, 189, 248, 0.2);
		font-size: 0.75rem;
		color: var(--color-text);
	}

	.banner-btn {
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		padding: 3px 8px;
		font-size: 0.72rem;
		font-weight: 600;
		cursor: pointer;
	}

	.link-status {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
		margin: 0 0 var(--space-xs);
	}

	.chip {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		padding: 2px 7px;
		border-radius: var(--radius-full);
		background: rgba(255, 255, 255, 0.06);
		color: var(--color-text-muted);
	}

	.chip.port.on {
		background: color-mix(in srgb, var(--color-connected) 18%, transparent);
		color: var(--color-connected);
	}

	.chip.kiss {
		background: color-mix(in srgb, var(--color-connected) 18%, transparent);
		color: var(--color-connected);
	}

	.chip.quiet {
		background: color-mix(in srgb, var(--color-warning) 18%, transparent);
		color: var(--color-warning);
	}

	.chip.error {
		background: color-mix(in srgb, var(--color-error) 18%, transparent);
		color: var(--color-error);
	}

	.counts {
		font-size: 0.72rem;
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}
</style>
