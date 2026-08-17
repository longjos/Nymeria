import type { SerialPortInfo, SerialProfile, TransportSettings, TransportStatus } from './types';

const MANUAL = '__manual__';

export function persistDevice(port: { name: string; stablePath?: string }): string {
	return port.stablePath || port.name;
}

/** Strip a Windows `\\.\` prefix so COM10 and \\.\COM10 compare equal. */
export function normalizeDevice(device: string): string {
	return device.replace(/^\\\\[.]\\/i, '');
}

export function formatMissingLabel(device: string): string {
	return `${normalizeDevice(device)} (not present)`;
}

export function mergePortOptions(live: SerialPortInfo[], configuredDevice: string): SerialPortInfo[] {
	const configured = normalizeDevice(configuredDevice ?? '');
	if (!configured) return live;

	const match = live.find(
		(p) => normalizeDevice(p.name) === configured || normalizeDevice(p.stablePath ?? '') === configured
	);
	if (match) return live;

	return [
		...live,
		{
			name: configured,
			label: formatMissingLabel(configured),
			present: false
		}
	];
}

export function inferProfile(
	port: SerialPortInfo | undefined,
	baud: number,
	profiles: SerialProfile[],
	previousId?: string
): string {
	const ids = new Set(profiles.map((p) => p.id));
	if (previousId === 'kenwood-thd7x-bt' && baud === 115200 && ids.has('kenwood-thd7x-bt')) {
		return 'kenwood-thd7x-bt';
	}
	if (port?.present && port.suggestedProfile && ids.has(port.suggestedProfile)) {
		return port.suggestedProfile;
	}
	if (baud === 115200 && ids.has('kenwood-thd7x-bt')) {
		return 'kenwood-thd7x-bt';
	}
	return ids.has('generic') ? 'generic' : (profiles[0]?.id ?? 'generic');
}

export function serialTransportId(all: TransportSettings[], index: number): string | null {
	const t = all[index];
	if (!t || t.type !== 'serial') return null;
	let n = 0;
	for (let i = 0; i < index; i++) {
		if (all[i].type === 'serial') n++;
	}
	return `serial-${n}`;
}

export function isLocalHost(hostname: string): boolean {
	const h = hostname.toLowerCase();
	return h === 'localhost' || h === '127.0.0.1' || h === '::1' || h === '[::1]';
}

export function emptyStateKind(hostOS: string): 'windows' | 'linux' | 'darwin' | 'other' {
	switch (hostOS) {
		case 'windows':
		case 'linux':
		case 'darwin':
			return hostOS;
		default:
			return 'other';
	}
}

export const MANUAL_PORT_VALUE = MANUAL;

export function highlightedPorts(ports: SerialPortInfo[]): SerialPortInfo[] {
	return ports.filter((p) => p.present !== false && p.highlight);
}

/** KISS-pipe liveness. Connected only means the OS opened the port. */
export type KissLinkState = 'disconnected' | 'error' | 'quiet' | 'kiss';

export function kissLinkState(s: TransportStatus | null | undefined): KissLinkState {
	if (!s) return 'disconnected';
	if (!s.connected && s.error) return 'error';
	if (!s.connected) return 'disconnected';
	if ((s.packetsRx ?? 0) > 0) return 'kiss';
	return 'quiet';
}

export function kissLinkLabel(state: KissLinkState): string {
	switch (state) {
		case 'kiss':
			return 'KISS heard';
		case 'quiet':
			return 'Quiet';
		case 'error':
			return 'Error';
		default:
			return 'Disconnected';
	}
}

export function kissLinkHint(state: KissLinkState): string {
	switch (state) {
		case 'kiss':
			return 'Decoded at least one KISS/AX.25 frame from this port.';
		case 'quiet':
			return 'Port is open. No KISS frames yet — radio may not be in KISS, or the frequency is silent.';
		case 'error':
			return 'The serial port is not open.';
		default:
			return 'Not connected.';
	}
}
