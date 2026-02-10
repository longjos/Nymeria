<script lang="ts">
	import type { Station } from '$lib/types';
	import { applyEquation, channelName, channelUnit } from '$lib/stores/telemetry';

	let {
		station,
		onClick
	}: {
		station: Station;
		onClick?: (callsign: string) => void;
	} = $props();

	let tel = $derived(station.telemetry);
	let params = $derived(station.telemetryParams ?? null);
	let key = $derived(station.ssid > 0 ? `${station.callsign}-${station.ssid}` : station.callsign);

	let ago = $derived.by(() => {
		const diff = Date.now() - new Date(station.lastHeard).getTime();
		const min = Math.floor(diff / 60000);
		if (min < 1) return 'just now';
		if (min < 60) return `${min}m ago`;
		const hrs = Math.floor(min / 60);
		if (hrs < 24) return `${hrs}h ago`;
		return `${Math.floor(hrs / 24)}d ago`;
	});

	function fmtVal(channel: number, raw: number): string {
		const val = applyEquation(params, channel, raw);
		return val === Math.floor(val) ? val.toString() : val.toFixed(1);
	}

	/** Return 8-bit digital value as array of booleans (bit 0 = index 0). */
	function digitalBits(d: number): boolean[] {
		const bits: boolean[] = [];
		for (let i = 0; i < 8; i++) {
			bits.push(((d >> i) & 1) === 1);
		}
		return bits;
	}
</script>

<button class="tel-card" onclick={() => onClick?.(key)}>
	<div class="tel-card-header">
		<span class="tel-callsign">{key}</span>
		<span class="tel-seq">#{tel?.seq ?? 0}</span>
		<span class="tel-ago">{ago}</span>
	</div>
	{#if tel}
		<div class="tel-card-body">
			<div class="tel-analogs">
				{#each [0, 1, 2, 3, 4] as ch}
					<div class="tel-analog">
						<span class="tel-analog-name">{channelName(params, ch)}</span>
						<span class="tel-analog-val">
							{fmtVal(ch, tel.analog[ch])}
							{#if channelUnit(params, ch)}
								<span class="tel-unit">{channelUnit(params, ch)}</span>
							{/if}
						</span>
					</div>
				{/each}
			</div>
			<div class="tel-digital">
				{#each digitalBits(tel.digital) as bit, i}
					<span class="tel-led" class:on={bit} title="Bit {i}"></span>
				{/each}
			</div>
		</div>
	{/if}
</button>

<style>
	.tel-card {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 10px 12px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background var(--duration-fast), border-color var(--duration-fast);
		text-align: left;
		color: var(--color-text);
		width: 100%;
	}

	.tel-card:hover {
		background: var(--color-primary);
	}

	.tel-card-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.tel-callsign {
		font-family: monospace;
		font-weight: 700;
		font-size: 0.85rem;
		color: var(--color-accent);
		flex: 1;
	}

	.tel-seq {
		font-family: monospace;
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.tel-ago {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.tel-card-body {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.tel-analogs {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 3px 12px;
	}

	.tel-analog {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 4px;
		font-size: 0.75rem;
	}

	.tel-analog-name {
		color: var(--color-text-muted);
		font-size: 0.7rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 70px;
	}

	.tel-analog-val {
		font-family: monospace;
		font-weight: 600;
	}

	.tel-unit {
		font-size: 0.6rem;
		color: var(--color-text-muted);
		font-weight: 400;
	}

	.tel-digital {
		display: flex;
		gap: 4px;
		padding-top: 2px;
	}

	.tel-led {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.15);
		transition: background 0.2s;
	}

	.tel-led.on {
		background: #22c55e;
		border-color: #22c55e;
		box-shadow: 0 0 4px rgba(34, 197, 94, 0.5);
	}
</style>
