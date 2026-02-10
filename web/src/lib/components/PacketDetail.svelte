<script lang="ts">
	import type { RawPacket } from '$lib/types';

	let { packet }: { packet: RawPacket } = $props();

	let copied = $state(false);

	function copyRaw() {
		navigator.clipboard.writeText(packet.raw).then(() => {
			copied = true;
			setTimeout(() => { copied = false; }, 1500);
		});
	}

	function formatAddress(addr: { call: string; ssid?: number; hBit?: boolean }): string {
		let s = addr.call;
		if (addr.ssid) s += `-${addr.ssid}`;
		if (addr.hBit) s += '*';
		return s;
	}

	function formatPath(path: Array<{ call: string; ssid?: number; hBit?: boolean }>): string {
		if (!path || path.length === 0) return 'none';
		return path.map(formatAddress).join(', ');
	}

	// Extract type-specific fields from the parsed packet
	let typeFields = $derived.by(() => {
		const p = packet.packet;
		const fields: Array<{ label: string; value: string }> = [];

		if (p.position) {
			const pos = p.position as Record<string, unknown>;
			fields.push({ label: 'Latitude', value: String(pos.lat) });
			fields.push({ label: 'Longitude', value: String(pos.lon) });
			if (pos.altitude) fields.push({ label: 'Altitude', value: `${pos.altitude}m` });
			if (pos.speed) fields.push({ label: 'Speed', value: `${Number(pos.speed).toFixed(1)} km/h` });
			if (pos.course) fields.push({ label: 'Course', value: `${pos.course}°` });
			if (pos.comment) fields.push({ label: 'Comment', value: String(pos.comment) });
		}

		if (p.message) {
			const msg = p.message as Record<string, unknown>;
			fields.push({ label: 'Addressee', value: String(msg.addressee) });
			if (msg.text) fields.push({ label: 'Text', value: String(msg.text) });
			if (msg.messageNo) fields.push({ label: 'Msg #', value: String(msg.messageNo) });
			if (msg.isAck) fields.push({ label: 'ACK', value: String(msg.ackMsgNo || '') });
			if (msg.isRej) fields.push({ label: 'REJ', value: String(msg.ackMsgNo || '') });
		}

		if (p.object) {
			const obj = p.object as Record<string, unknown>;
			fields.push({ label: 'Name', value: String(obj.name) });
			fields.push({ label: 'Live', value: obj.live ? 'Yes' : 'Killed' });
		}

		if (p.item) {
			const item = p.item as Record<string, unknown>;
			fields.push({ label: 'Name', value: String(item.name) });
			fields.push({ label: 'Live', value: item.live ? 'Yes' : 'Killed' });
		}

		if (p.weather) {
			const wx = p.weather as Record<string, unknown>;
			if (wx.temperature != null) fields.push({ label: 'Temperature', value: `${Number(wx.temperature).toFixed(1)}°C` });
			if (wx.windSpeed != null) fields.push({ label: 'Wind Speed', value: `${Number(wx.windSpeed).toFixed(1)} m/s` });
			if (wx.windDir != null) fields.push({ label: 'Wind Dir', value: `${wx.windDir}°` });
			if (wx.windGust != null) fields.push({ label: 'Wind Gust', value: `${Number(wx.windGust).toFixed(1)} m/s` });
			if (wx.humidity != null) fields.push({ label: 'Humidity', value: `${wx.humidity}%` });
			if (wx.pressure != null) fields.push({ label: 'Pressure', value: `${Number(wx.pressure).toFixed(1)} hPa` });
		}

		if (p.status) {
			const st = p.status as Record<string, unknown>;
			fields.push({ label: 'Status', value: String(st.text) });
			if (st.maidenhead) fields.push({ label: 'Grid', value: String(st.maidenhead) });
		}

		if (p.telemetry) {
			const tel = p.telemetry as Record<string, unknown>;
			fields.push({ label: 'Seq', value: String(tel.seq) });
			if (Array.isArray(tel.analog)) {
				fields.push({ label: 'Analog', value: (tel.analog as number[]).map((v) => v.toFixed(0)).join(', ') });
			}
			fields.push({ label: 'Digital', value: String(tel.digital) });
		}

		if (p.micE) {
			const me = p.micE as Record<string, unknown>;
			if (me.micEMsg) fields.push({ label: 'Mic-E Msg', value: String(me.micEMsg) });
			if (me.radioModel) fields.push({ label: 'Radio', value: String(me.radioModel) });
		}

		if (p.query) {
			fields.push({ label: 'Query', value: String(p.query) });
		}

		if (p.frequency) {
			const freq = p.frequency as Record<string, unknown>;
			fields.push({ label: 'Frequency', value: `${freq.freq} MHz` });
			if (freq.tone) fields.push({ label: 'Tone', value: `${freq.tone} Hz` });
			if (freq.offset) fields.push({ label: 'Offset', value: `${freq.offset} MHz` });
		}

		return fields;
	});
</script>

<div class="packet-detail">
	<div class="raw-block">
		<pre>{packet.raw}</pre>
		<button class="copy-btn" onclick={copyRaw} title="Copy raw packet">
			{#if copied}
				<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><path d="M3 8l3 3 7-7" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
			{:else}
				<svg width="14" height="14" viewBox="0 0 16 16" fill="none"><rect x="5" y="5" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.3"/><path d="M3 11V3h8" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>
			{/if}
		</button>
	</div>

	<table class="fields-table">
		<tbody>
			<tr><td class="field-label">From</td><td class="field-value">{formatAddress(packet.from)}</td></tr>
			<tr><td class="field-label">To</td><td class="field-value">{formatAddress(packet.to)}</td></tr>
			<tr><td class="field-label">Path</td><td class="field-value">{formatPath(packet.path)}</td></tr>
			<tr><td class="field-label">Type</td><td class="field-value"><span class="type-pill {packet.packetType}">{packet.packetType}</span></td></tr>
			{#each typeFields as field}
				<tr><td class="field-label">{field.label}</td><td class="field-value">{field.value}</td></tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.packet-detail {
		padding: 8px 12px 12px;
		border-top: 1px solid var(--color-primary);
		background: rgba(255, 255, 255, 0.02);
	}

	.raw-block {
		position: relative;
		margin-bottom: 8px;
	}

	.raw-block pre {
		margin: 0;
		padding: 8px 10px;
		background: var(--color-bg);
		border-radius: var(--radius-sm);
		font-family: 'JetBrains Mono', 'Fira Code', monospace;
		font-size: 0.75rem;
		line-height: 1.5;
		color: var(--color-text);
		white-space: pre-wrap;
		word-break: break-all;
	}

	.copy-btn {
		position: absolute;
		top: 4px;
		right: 4px;
		padding: 4px;
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		opacity: 0.7;
		transition: opacity 0.15s;
	}

	.copy-btn:hover {
		opacity: 1;
		color: var(--color-text);
	}

	.fields-table {
		width: 100%;
		border-collapse: collapse;
	}

	.fields-table tr {
		border-bottom: 1px solid rgba(255, 255, 255, 0.04);
	}

	.fields-table tr:last-child {
		border-bottom: none;
	}

	.field-label {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		padding: 3px 8px 3px 0;
		white-space: nowrap;
		vertical-align: top;
		width: 80px;
	}

	.field-value {
		font-size: 0.78rem;
		color: var(--color-text);
		font-family: 'JetBrains Mono', 'Fira Code', monospace;
		padding: 3px 0;
		word-break: break-all;
	}

	.type-pill {
		display: inline-block;
		padding: 1px 6px;
		border-radius: 8px;
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.type-pill.position { background: rgba(var(--accent-rgb, 0, 191, 255), 0.15); color: var(--color-accent); }
	.type-pill.message { background: rgba(96, 165, 250, 0.15); color: #60a5fa; }
	.type-pill.weather { background: rgba(245, 158, 11, 0.15); color: #f59e0b; }
	.type-pill.telemetry { background: rgba(168, 85, 247, 0.15); color: #a855f7; }
	.type-pill.object, .type-pill.item { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
	.type-pill.micE { background: rgba(236, 72, 153, 0.15); color: #ec4899; }
	.type-pill.status, .type-pill.query, .type-pill.unknown, .type-pill.thirdParty { background: rgba(148, 163, 184, 0.15); color: #94a3b8; }
</style>
