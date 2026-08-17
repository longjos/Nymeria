import { describe, it, expect } from 'vitest';
import {
	tncKey,
	normalizeKissHost,
	mergeTncOptions,
	kissTcpTransportId,
	parseTncValue,
	persistTncValue
} from './kissTncs';
import type { KissTncInfo } from './types';

const dw: KissTncInfo = {
	name: 'Dire Wolf on radiopi',
	label: 'Dire Wolf on radiopi — 192.168.1.40:8001',
	host: '192.168.1.40',
	port: 8001,
	source: 'mdns',
	highlight: true
};

describe('normalizeKissHost', () => {
	it('collapses loopback names', () => {
		expect(normalizeKissHost('127.0.0.1')).toBe('localhost');
		expect(normalizeKissHost('LOCALHOST')).toBe('localhost');
	});
});

describe('tncKey', () => {
	it('defaults port 8001', () => {
		expect(tncKey('localhost', undefined)).toBe('localhost:8001');
	});
});

describe('mergeTncOptions', () => {
	it('keeps a saved host that is not live', () => {
		const merged = mergeTncOptions([dw], '10.0.0.9', 8001);
		expect(merged.some((t) => t.host === '10.0.0.9' && t.present === false)).toBe(true);
	});

	it('does not duplicate localhost vs 127.0.0.1', () => {
		const local: KissTncInfo = {
			name: 'This computer',
			label: 'This computer (localhost:8001)',
			host: 'localhost',
			port: 8001,
			source: 'local',
			local: true
		};
		const merged = mergeTncOptions([local], '127.0.0.1', 8001);
		expect(merged).toHaveLength(1);
	});
});

describe('kissTcpTransportId', () => {
	it('numbers kisstcp independently', () => {
		const all = [{ type: 'aprsis' }, { type: 'kisstcp' }, { type: 'serial' }, { type: 'kisstcp' }];
		expect(kissTcpTransportId(all, 1)).toBe('kisstcp-0');
		expect(kissTcpTransportId(all, 3)).toBe('kisstcp-1');
	});
});

describe('parseTncValue', () => {
	it('splits host:port', () => {
		expect(parseTncValue(persistTncValue(dw))).toEqual({ host: '192.168.1.40', port: 8001 });
	});
});
