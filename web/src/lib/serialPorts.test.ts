import { describe, it, expect } from 'vitest';
import {
	persistDevice,
	normalizeDevice,
	mergePortOptions,
	inferProfile,
	serialTransportId,
	isLocalHost,
	formatMissingLabel,
	kissLinkState,
	kissLinkLabel
} from './serialPorts';
import type { SerialPortInfo, SerialProfile } from './types';

const profiles: SerialProfile[] = [
	{ id: 'generic', label: 'Generic', baud: 9600, help: '' },
	{ id: 'kenwood-thd7x-usb', label: 'Kenwood USB', baud: 9600, help: '' },
	{ id: 'kenwood-thd7x-bt', label: 'Kenwood BT', baud: 115200, help: '' },
	{ id: 'mobilinkd', label: 'Mobilinkd', baud: 9600, help: '' }
];

const kenwood: SerialPortInfo = {
	name: 'COM5',
	label: 'TH-D74 (COM5)',
	present: true,
	suggestedProfile: 'kenwood-thd7x-usb',
	highlight: true
};

describe('persistDevice', () => {
	it('prefers stablePath over name', () => {
		expect(persistDevice({ name: '/dev/ttyACM0', stablePath: '/dev/serial/by-id/usb-Kenwood' })).toBe(
			'/dev/serial/by-id/usb-Kenwood'
		);
	});

	it('falls back to name', () => {
		expect(persistDevice({ name: 'COM5' })).toBe('COM5');
	});
});

describe('normalizeDevice', () => {
	it('strips Windows prefix', () => {
		expect(normalizeDevice('\\\\.\\COM10')).toBe('COM10');
	});

	it('leaves a bare COM name', () => {
		expect(normalizeDevice('COM10')).toBe('COM10');
	});
});

describe('mergePortOptions', () => {
	it('keeps configured COM12 as not-present', () => {
		const merged = mergePortOptions([kenwood], 'COM12');
		const missing = merged.find((p) => p.name === 'COM12');
		expect(missing).toBeTruthy();
		expect(missing?.present).toBe(false);
		expect(missing?.label).toBe(formatMissingLabel('COM12'));
	});

	it('does not duplicate when live name matches', () => {
		const merged = mergePortOptions([kenwood], 'COM5');
		expect(merged.filter((p) => p.name === 'COM5')).toHaveLength(1);
		expect(merged[0].present).not.toBe(false);
	});

	it('treats saved \\\\.\\COM10 as present when live name is COM10', () => {
		const live: SerialPortInfo = { name: 'COM10', label: 'COM10', present: true };
		const merged = mergePortOptions([live], '\\\\.\\COM10');
		expect(merged).toHaveLength(1);
		expect(merged[0].name).toBe('COM10');
	});

	it('matches configured by-id via stablePath', () => {
		const live: SerialPortInfo = {
			name: '/dev/ttyACM0',
			label: 'Kenwood',
			present: true,
			stablePath: '/dev/serial/by-id/usb-Kenwood'
		};
		const merged = mergePortOptions([live], '/dev/serial/by-id/usb-Kenwood');
		expect(merged).toHaveLength(1);
	});
});

describe('inferProfile', () => {
	it('uses suggested Kenwood USB profile', () => {
		expect(inferProfile(kenwood, 9600, profiles)).toBe('kenwood-thd7x-usb');
	});

	it('uses Kenwood BT when baud is 115200 and no USB match', () => {
		expect(inferProfile(undefined, 115200, profiles)).toBe('kenwood-thd7x-bt');
	});

	it('keeps Kenwood BT when previous id is BT even if a USB port is selected', () => {
		expect(inferProfile(kenwood, 115200, profiles, 'kenwood-thd7x-bt')).toBe('kenwood-thd7x-bt');
	});

	it('falls back to generic', () => {
		expect(inferProfile(undefined, 9600, profiles)).toBe('generic');
	});
});

describe('serialTransportId', () => {
	it('numbers serial transports independently of other types', () => {
		const all = [
			{ type: 'aprsis' },
			{ type: 'serial' },
			{ type: 'kisstcp' },
			{ type: 'serial' }
		];
		expect(serialTransportId(all, 1)).toBe('serial-0');
		expect(serialTransportId(all, 3)).toBe('serial-1');
	});
});

describe('isLocalHost', () => {
	it('recognizes loopback names', () => {
		expect(isLocalHost('127.0.0.1')).toBe(true);
		expect(isLocalHost('localhost')).toBe(true);
		expect(isLocalHost('::1')).toBe(true);
	});

	it('rejects LAN hosts', () => {
		expect(isLocalHost('192.168.1.5')).toBe(false);
	});
});

describe('kissLinkState', () => {
	it('is disconnected when status is missing', () => {
		expect(kissLinkState(null)).toBe('disconnected');
	});

	it('is error when the port failed to stay open', () => {
		expect(
			kissLinkState({
				id: 'serial-0',
				type: 'serial',
				connected: false,
				error: 'access denied',
				packetsRx: 0,
				packetsTx: 0
			})
		).toBe('error');
	});

	it('is quiet when the COM port is open but no KISS has been decoded', () => {
		expect(
			kissLinkState({
				id: 'serial-0',
				type: 'serial',
				connected: true,
				packetsRx: 0,
				packetsTx: 0
			})
		).toBe('quiet');
	});

	it('is kiss after at least one inbound frame', () => {
		expect(
			kissLinkState({
				id: 'serial-0',
				type: 'serial',
				connected: true,
				packetsRx: 1,
				packetsTx: 0
			})
		).toBe('kiss');
	});

	it('labels match the chip copy', () => {
		expect(kissLinkLabel('quiet')).toBe('Quiet');
		expect(kissLinkLabel('kiss')).toBe('KISS heard');
		expect(kissLinkLabel('error')).toBe('Error');
		expect(kissLinkLabel('disconnected')).toBe('Disconnected');
	});
});
