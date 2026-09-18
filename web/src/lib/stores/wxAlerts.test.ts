import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
	wxAlertsById,
	wxLinkStatus,
	wxFootprint,
	wxPolicy,
	wxAcked,
	wxMuted,
	wxInterruptQueue,
	wxInAreaAlerts,
	wxActiveInterrupt,
	wxInterruptMore,
	wxUnackedCount,
	applyAlerts,
	handleWxMessage,
	muteAlert
} from './wxAlerts';
import { toasts } from './toast';
import type { WxAlert, WxLinkStatus, WxEffectivePolicy } from '$lib/types';
import { wxAlertFixtureAlerts, wxAlertFixtureSnapshot } from '$lib/data/wxAlertFixture';

const STATUS: WxLinkStatus = { ...wxAlertFixtureSnapshot.status };
const POLICY: WxEffectivePolicy = { ...wxAlertFixtureSnapshot.policy };

function reset(): void {
	wxAlertsById.set(new Map());
	wxLinkStatus.set(STATUS);
	wxFootprint.set(null);
	wxPolicy.set(null);
	wxAcked.set(new Map());
	wxMuted.set(new Set());
	wxInterruptQueue.set([]);
	toasts.set([]);
}

beforeEach(() => {
	reset();
});

function base(id: string, updatedAt: string, overrides: Partial<WxAlert> = {}): WxAlert {
	const tor = wxAlertFixtureAlerts[0];
	return {
		...tor,
		id,
		updatedAt,
		firstSeenAt: updatedAt,
		fetchedAt: updatedAt,
		notifyReason: 'new',
		notifyClass: 'toast',
		...overrides
	};
}

describe('applyAlerts', () => {
	it('handles alerts: null without throwing and empties the in-area list', () => {
		expect(() => applyAlerts(null, STATUS, null, POLICY, 'poll')).not.toThrow();
		expect(get(wxInAreaAlerts)).toEqual([]);
	});

	it('dedupes by updatedAt — the same version processed twice toasts once', () => {
		const a = base('dedupe-1', '2026-09-17T20:00:00Z');
		applyAlerts([a], STATUS, null, POLICY, 'poll');
		applyAlerts([a], STATUS, null, POLICY, 'poll');
		expect(get(toasts).length).toBe(1);
	});

	it('processes a later version of the same alert (different updatedAt) again', () => {
		const v1 = base('dedupe-2', '2026-09-17T20:00:00Z');
		const v2 = base('dedupe-2', '2026-09-17T20:05:00Z', { notifyReason: 'update' });
		applyAlerts([v1], STATUS, null, POLICY, 'poll');
		applyAlerts([v2], STATUS, null, POLICY, 'poll');
		expect(get(toasts).length).toBe(2);
	});

	describe('restore rule', () => {
		it('re-arms an unacked active interrupt-class alert', () => {
			const a = base('restore-1', '2026-09-17T20:00:00Z', { notifyClass: 'interrupt', state: 'active' });
			applyAlerts([a], STATUS, null, POLICY, 'restore');
			expect(get(wxInterruptQueue)).toEqual(['restore-1']);
			expect(get(toasts).length).toBe(0); // restore never toasts
		});

		it('does not re-arm an alert already acked for the net', () => {
			const a = base('restore-2', '2026-09-17T20:00:00Z', {
				notifyClass: 'interrupt',
				state: 'active',
				ackedForNet: { userId: 'u1', userName: 'Alice', callsign: 'W8ABC', at: '2026-09-17T20:01:00Z' }
			});
			applyAlerts([a], STATUS, null, POLICY, 'restore');
			expect(get(wxInterruptQueue)).toEqual([]);
		});

		it('never toasts a non-interrupt alert on restore, however loud its class', () => {
			const a = base('restore-3', '2026-09-17T20:00:00Z', { notifyClass: 'toast' });
			applyAlerts([a], STATUS, null, POLICY, 'restore');
			expect(get(toasts).length).toBe(0);
			expect(get(wxInterruptQueue)).toEqual([]);
		});
	});

	describe('ended toasts', () => {
		it('toasts an ended alert that was IN and watch-tier or above', () => {
			const a = base('ended-1', '2026-09-17T21:00:00Z', { tier: 'watch', notifyReason: 'ended', state: 'expired', proximity: 'in' });
			applyAlerts([a], STATUS, null, POLICY, 'poll');
			expect(get(toasts).length).toBe(1);
		});

		it('does not toast an ended advisory (below watch tier)', () => {
			const a = base('ended-2', '2026-09-17T21:00:00Z', { tier: 'advisory', notifyReason: 'ended', state: 'expired', proximity: 'in' });
			applyAlerts([a], STATUS, null, POLICY, 'poll');
			expect(get(toasts).length).toBe(0);
		});

		it('does not toast an ended NEAR alert', () => {
			const a = base('ended-3', '2026-09-17T21:00:00Z', { tier: 'warning', notifyReason: 'ended', state: 'expired', proximity: 'near' });
			applyAlerts([a], STATUS, null, POLICY, 'poll');
			expect(get(toasts).length).toBe(0);
		});
	});

	describe('mute vs escalation', () => {
		it('suppresses a non-escalated update once muted, but an escalation still notifies', () => {
			const v1 = base('mute-1', '2026-09-17T20:00:00Z', { notifyClass: 'toast', notifyReason: 'new' });
			applyAlerts([v1], STATUS, null, POLICY, 'poll');
			expect(get(toasts).length).toBe(1);

			muteAlert('mute-1'); // toast-class alert — mute is allowed (only interrupt refuses)

			const v2 = base('mute-1', '2026-09-17T20:05:00Z', { notifyClass: 'toast', notifyReason: 'update' });
			applyAlerts([v2], STATUS, null, POLICY, 'poll');
			expect(get(toasts).length).toBe(1); // suppressed

			const v3 = base('mute-1', '2026-09-17T20:10:00Z', { notifyClass: 'toast', notifyReason: 'escalated' });
			applyAlerts([v3], STATUS, null, POLICY, 'poll');
			expect(get(toasts).length).toBe(2); // escalation still notifies
		});

		it('refuses to mute an interrupt-class alert (no-op)', () => {
			const a = base('mute-2', '2026-09-17T20:00:00Z', { notifyClass: 'interrupt', notifyReason: 'new' });
			applyAlerts([a], STATUS, null, POLICY, 'poll');
			muteAlert('mute-2');
			expect(get(wxMuted).has('mute-2')).toBe(false);
		});
	});

	it('coalesces more than one simultaneous toast into a single summary toast', () => {
		const a = base('coalesce-1', '2026-09-17T20:00:00Z', { notifyClass: 'toast' });
		const b = base('coalesce-2', '2026-09-17T20:00:00Z', { notifyClass: 'toast' });
		const c = base('coalesce-3', '2026-09-17T20:00:00Z', { notifyClass: 'toast' });
		applyAlerts([a, b, c], STATUS, null, POLICY, 'poll');
		expect(get(toasts)).toHaveLength(1);
		expect(get(toasts)[0].message).toBe('3 new NWS alerts in your area');
	});
});

describe('wx_alert_ack_net (decision 5)', () => {
	it('dequeues the acked alert and advances the banner to the next queued one', () => {
		const a = base('ack-net-1', '2026-09-17T20:00:00Z', { notifyClass: 'interrupt' });
		const b = base('ack-net-2', '2026-09-17T20:00:01Z', { notifyClass: 'interrupt' });
		applyAlerts([a, b], STATUS, null, POLICY, 'restore');

		expect(get(wxInterruptQueue)).toEqual(['ack-net-1', 'ack-net-2']);
		expect(get(wxActiveInterrupt)?.id).toBe('ack-net-1');
		expect(get(wxInterruptMore)).toBe(1);

		handleWxMessage('wx_alert_ack_net', {
			alertId: 'ack-net-1',
			netId: 'net-1',
			ackedForNet: { userId: 'u1', userName: 'Alice', callsign: 'W8ABC', at: '2026-09-17T20:10:00Z' }
		});

		expect(get(wxInterruptQueue)).toEqual(['ack-net-2']);
		expect(get(wxActiveInterrupt)?.id).toBe('ack-net-2');
		expect(get(wxAlertsById).get('ack-net-1')?.ackedForNet?.callsign).toBe('W8ABC');
	});

	it('leaves the banner empty once the only queued alert is acked for the net', () => {
		const a = base('ack-net-solo', '2026-09-17T20:00:00Z', { notifyClass: 'interrupt' });
		applyAlerts([a], STATUS, null, POLICY, 'restore');
		expect(get(wxActiveInterrupt)?.id).toBe('ack-net-solo');

		handleWxMessage('wx_alert_ack_net', {
			alertId: 'ack-net-solo',
			netId: 'net-1',
			ackedForNet: { userId: 'u1', userName: 'Alice', callsign: 'W8ABC', at: '2026-09-17T20:10:00Z' }
		});

		expect(get(wxActiveInterrupt)).toBeNull();
	});
});

describe('wxUnackedCount', () => {
	it('counts watch tier and up, excluding advisories and alerts already acked for the net', () => {
		const warning = base('count-warning', '2026-09-17T20:00:00Z', { tier: 'warning' });
		const watch = base('count-watch', '2026-09-17T20:00:01Z', { tier: 'watch' });
		const advisory = base('count-advisory', '2026-09-17T20:00:02Z', { tier: 'advisory' });
		const ackedWarning = base('count-acked', '2026-09-17T20:00:03Z', {
			tier: 'warning',
			ackedForNet: { userId: 'u1', userName: 'Alice', callsign: 'W8ABC', at: '2026-09-17T20:01:00Z' }
		});

		applyAlerts([warning, watch, advisory, ackedWarning], STATUS, null, POLICY, 'poll');

		expect(get(wxUnackedCount)).toBe(2);
	});
});
