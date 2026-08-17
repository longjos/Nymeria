import type { KissTncInfo, TransportSettings } from './types';

const MANUAL = '__manual__';

export const MANUAL_TNC_VALUE = MANUAL;

export function tncKey(host: string, port: number | undefined): string {
	const h = normalizeKissHost(host);
	const p = port && port > 0 ? port : 8001;
	return `${h}:${p}`;
}

export function normalizeKissHost(host: string): string {
	const h = (host ?? '').trim().replace(/\.$/, '').toLowerCase();
	if (h === '127.0.0.1' || h === '::1' || h === '[::1]' || h === 'localhost') {
		return 'localhost';
	}
	return h;
}

export function persistTncValue(t: KissTncInfo): string {
	return tncKey(t.host, t.port);
}

export function mergeTncOptions(
	live: KissTncInfo[],
	host: string | undefined,
	port: number | undefined
): KissTncInfo[] {
	const configured = (host ?? '').trim();
	if (!configured) return live;
	const key = tncKey(configured, port);
	const match = live.find((t) => persistTncValue(t) === key);
	if (match) return live;
	const p = port && port > 0 ? port : 8001;
	return [
		...live,
		{
			name: `${configured}:${p}`,
			label: `${configured}:${p} (not present)`,
			host: configured,
			port: p,
			source: 'saved',
			present: false
		}
	];
}

export function kissTcpTransportId(all: TransportSettings[], index: number): string | null {
	const t = all[index];
	if (!t || t.type !== 'kisstcp') return null;
	let n = 0;
	for (let i = 0; i < index; i++) {
		if (all[i].type === 'kisstcp') n++;
	}
	return `kisstcp-${n}`;
}

export function parseTncValue(value: string): { host: string; port: number } | null {
	if (!value || value === MANUAL) return null;
	const idx = value.lastIndexOf(':');
	if (idx <= 0) return { host: value, port: 8001 };
	const port = Number(value.slice(idx + 1));
	if (!Number.isFinite(port) || port <= 0) return { host: value, port: 8001 };
	return { host: value.slice(0, idx), port };
}
