import { describe, it, expect } from 'vitest';
import {
	nextLegAction,
	legSkips,
	legExceptions,
	activeLegs,
	legStateSince,
	vehicleJobs,
	unitLabel,
	sortVehicles,
	findUnit,
	jobExtent,
	allSagExtent,
	vehicleExtent,
	padSinglePoint,
	loadDraft,
	loadPlan,
	loadSummary,
	deliverDraft,
	slotName
} from './sagDockModel';
import type { SAGLeg, SAGRequest, SAGSlot, SAGVehicleStatus } from './types';

function slot(over: Partial<SAGSlot> = {}): SAGSlot {
	return {
		id: over.id ?? 's1',
		bib: over.bib,
		riderName: over.riderName,
		hasBike: over.hasBike ?? true,
		bike: over.bike ?? 'with_rider',
		disposition: over.disposition ?? 'waiting',
		legId: over.legId,
		updatedAt: '2026-09-23T12:00:00Z'
	};
}

function leg(over: Partial<SAGLeg> = {}): SAGLeg {
	return {
		id: over.id ?? 'L1',
		vehicleCheckInId: over.vehicleCheckInId ?? 'v1',
		vehicleLabel: over.vehicleLabel ?? 'SAG 2',
		slotIds: over.slotIds ?? ['s1'],
		status: over.status ?? 'dispatched',
		overcommitted: false,
		dispatchedAt: over.dispatchedAt ?? '2026-09-23T12:00:00Z',
		enrouteAt: over.enrouteAt,
		onSceneAt: over.onSceneAt,
		loadedAt: over.loadedAt,
		deliveredAt: over.deliveredAt,
		releasedAt: over.releasedAt
	};
}

function req(over: Partial<SAGRequest> = {}): SAGRequest {
	return {
		id: over.id ?? 'r1',
		netId: 'n1',
		sequence: over.sequence ?? 7,
		pickup: over.pickup ?? { kind: 'course', mileMarker: 30 },
		dropoff: over.dropoff ?? { kind: 'next_reststop' },
		reason: 'mechanical',
		priority: 'high',
		status: over.status ?? 'assigned',
		needsVehicle: false,
		slots: over.slots ?? [slot({ legId: 'L1' })],
		legs: over.legs ?? [leg()],
		requestedBy: '',
		notes: '',
		createdAt: over.createdAt ?? '2026-09-23T11:50:00Z',
		updatedAt: '2026-09-23T12:00:00Z'
	};
}

function vehicle(over: Partial<SAGVehicleStatus> = {}): SAGVehicleStatus {
	return {
		netId: 'n1',
		checkInId: over.checkInId ?? 'v1',
		seats: 4,
		rackSlots: 2,
		notes: '',
		updatedAt: '',
		callsign: over.callsign ?? 'W4SGA',
		tacticalCall: over.tacticalCall ?? '',
		checkInStatus: over.checkInStatus ?? 'available',
		committedSeats: 0,
		committedRacks: 0,
		availableSeats: 4,
		availableRacks: 2,
		activeLegIds: [],
		activeRequestIds: []
	};
}

describe('nextLegAction — the one primary per rung', () => {
	it.each([
		['dispatched', 'advance', 'enroute', 'En route', false],
		['enroute', 'advance', 'onscene', 'On scene', false],
		['onscene', 'load', 'loaded', 'Loaded', true],
		['loaded', 'deliver', 'delivered', 'Delivered', true]
	])('%s -> %s %s', (status, verb, to, label, confirm) => {
		const a = nextLegAction(status)!;
		expect(a.verb).toBe(verb);
		expect(a.to).toBe(to);
		expect(a.label).toBe(label);
		expect(a.needsConfirm).toBe(confirm);
	});

	it.each(['delivered', 'released', 'bogus'])('%s has no next action', (s) => {
		expect(nextLegAction(s)).toBeNull();
	});
});

describe('legSkips — forward skips, never the primary, never backwards', () => {
	it('dispatched can skip to on scene or straight to loaded', () => {
		expect(legSkips('dispatched').map((a) => a.to)).toEqual(['onscene', 'loaded']);
	});
	it('en route can skip to loaded only', () => {
		expect(legSkips('enroute').map((a) => a.to)).toEqual(['loaded']);
	});
	it('on scene, loaded and terminal legs have nothing to skip', () => {
		for (const s of ['onscene', 'loaded', 'delivered', 'released']) expect(legSkips(s)).toEqual([]);
	});
	it('a skip is never the same as the primary', () => {
		for (const s of ['dispatched', 'enroute']) {
			const p = nextLegAction(s)!;
			expect(legSkips(s).some((k) => k.to === p.to)).toBe(false);
		}
	});
	it('a skip to loaded is the load verb (the server refuses loaded via advance)', () => {
		expect(legSkips('dispatched').find((k) => k.to === 'loaded')!.verb).toBe('load');
	});
});

describe('legExceptions — per-rider outcomes the server will accept', () => {
	it('a waiting rider on the leg can self-resolve, decline or be not found', () => {
		const r = req({ slots: [slot({ id: 's1', bib: '512', legId: 'L1' })] });
		const ex = legExceptions(r, r.legs[0]);
		expect(ex).toHaveLength(1);
		expect(ex[0].slotId).toBe('s1');
		expect(ex[0].name).toBe('Bib 512');
		expect(ex[0].options).toEqual(['self_resolved', 'declined', 'not_found']);
	});
	it('a loaded rider can only be handed off', () => {
		const r = req({
			slots: [slot({ id: 's1', disposition: 'loaded', legId: 'L1' })],
			legs: [leg({ status: 'loaded' })]
		});
		expect(legExceptions(r, r.legs[0])[0].options).toEqual(['handed_off']);
	});
	it('riders on other legs, and terminal riders, are not offered', () => {
		const r = req({
			slots: [
				slot({ id: 's1', legId: 'L2' }),
				slot({ id: 's2', legId: 'L1', disposition: 'delivered' }),
				slot({ id: 's3', legId: undefined })
			]
		});
		expect(legExceptions(r, r.legs[0])).toEqual([]);
	});
});

describe('activeLegs / legStateSince', () => {
	it('keeps dispatched..loaded, in dispatch order', () => {
		const r = req({
			legs: [
				leg({ id: 'b', status: 'loaded', dispatchedAt: '2026-09-23T12:05:00Z' }),
				leg({ id: 'x', status: 'released' }),
				leg({ id: 'a', status: 'enroute', dispatchedAt: '2026-09-23T12:01:00Z' }),
				leg({ id: 'd', status: 'delivered' })
			]
		});
		expect(activeLegs(r).map((l) => l.id)).toEqual(['a', 'b']);
	});
	it('reports when the leg entered its current state', () => {
		expect(legStateSince(leg({ status: 'enroute', enrouteAt: 'E' }))).toBe('E');
		expect(legStateSince(leg({ status: 'onscene', onSceneAt: 'O' }))).toBe('O');
		expect(legStateSince(leg({ status: 'loaded', loadedAt: 'L' }))).toBe('L');
		expect(legStateSince(leg({ status: 'dispatched', dispatchedAt: 'D' }))).toBe('D');
		// A skipped rung has no timestamp of its own: fall back to dispatch.
		expect(legStateSince(leg({ status: 'onscene', dispatchedAt: 'D' }))).toBe('D');
	});
});

describe('vehicleJobs — a van with two jobs shows both', () => {
	it('groups active legs by vehicle, skipping terminal requests', () => {
		const r1 = req({ id: 'r1', legs: [leg({ id: 'L1', vehicleCheckInId: 'v1', dispatchedAt: '2026-09-23T12:00:00Z' })] });
		const r2 = req({
			id: 'r2',
			legs: [
				leg({ id: 'L2', vehicleCheckInId: 'v1', status: 'dispatched', dispatchedAt: '2026-09-23T12:10:00Z' }),
				leg({ id: 'L3', vehicleCheckInId: 'v2', status: 'released' })
			]
		});
		const r3 = req({ id: 'r3', status: 'complete', legs: [leg({ id: 'L4', vehicleCheckInId: 'v2', status: 'loaded' })] });
		const jobs = vehicleJobs([r2, r1, r3]);
		expect(jobs.get('v1')!.map((j) => j.leg.id)).toEqual(['L1', 'L2']);
		expect(jobs.get('v2')).toBeUndefined();
	});
});

describe('unit naming and ordering', () => {
	it('prefers the tactical call', () => {
		expect(unitLabel(vehicle({ tacticalCall: 'SAG 2', callsign: 'W4SGA' }))).toBe('SAG 2');
		expect(unitLabel(vehicle({ tacticalCall: '', callsign: 'W4SGA' }))).toBe('W4SGA');
	});
	it('sorts naturally and stably by name, so SAG 10 follows SAG 9', () => {
		const vs = ['SAG 10', 'SAG 2', 'SAG 9', 'KG4YFA-4'].map((t, i) => vehicle({ checkInId: `v${i}`, tacticalCall: t }));
		expect(sortVehicles(vs).map(unitLabel)).toEqual(['KG4YFA-4', 'SAG 2', 'SAG 9', 'SAG 10']);
	});
});

describe('findUnit — jump to the unit the driver just named', () => {
	const vs = [
		vehicle({ checkInId: 'a', tacticalCall: 'SAG 2', callsign: 'W4SGA' }),
		vehicle({ checkInId: 'b', tacticalCall: 'SAG 12', callsign: 'KG4YFA-4' }),
		vehicle({ checkInId: 'c', tacticalCall: '', callsign: 'N4XYZ' })
	];
	it('matches ignoring case, spaces and hyphens', () => {
		expect(findUnit('sag2', vs)).toBe('a');
		expect(findUnit('kg4yfa4', vs)).toBe('b');
	});
	it('a bare number means the unit numbered that, not any unit containing it', () => {
		expect(findUnit('2', vs)).toBe('a');
		expect(findUnit('12', vs)).toBe('b');
	});
	it('falls back to prefix then contains, on tactical or callsign', () => {
		expect(findUnit('n4', vs)).toBe('c');
		expect(findUnit('sga', vs)).toBe('a');
	});
	it('returns null for no match or an empty query', () => {
		expect(findUnit('zzz', vs)).toBeNull();
		expect(findUnit('  ', vs)).toBeNull();
	});
});

describe('extents', () => {
	const placed = (lat: number, lon: number) => ({ placed: true as const, lat, lon, label: 'x' });
	const unplaced = { placed: false as const, label: 'x' };
	const geo = [
		{ vehicle: vehicle({ checkInId: 'v1' }), lat: 36, lon: -86, lastHeard: 'T' },
		{ vehicle: vehicle({ checkInId: 'v2' }), lat: 37, lon: -87 },
		{ vehicle: vehicle({ checkInId: 'v3' }) },
		{ vehicle: vehicle({ checkInId: 'v4', checkInStatus: 'released' }), lat: 1, lon: 1 }
	];

	it('a job is its pickup, its dropoff and its active vehicles', () => {
		const r = req({ legs: [leg({ vehicleCheckInId: 'v1' }), leg({ id: 'L9', vehicleCheckInId: 'v2', status: 'released' })] });
		const pts = jobExtent({ request: r, pickup: placed(35, -85), dropoff: placed(35.5, -85.5) }, geo);
		expect(pts).toEqual([
			{ lat: 35, lon: -85 },
			{ lat: 35.5, lon: -85.5 },
			{ lat: 36, lon: -86 }
		]);
	});
	it('an unplaceable pickup moves the map nowhere', () => {
		const r = req();
		expect(jobExtent({ request: r, pickup: unplaced, dropoff: placed(1, 1) }, geo)).toEqual([]);
	});
	it('a vehicle with no fix adds nothing', () => {
		const r = req({ legs: [leg({ vehicleCheckInId: 'v3' })] });
		expect(jobExtent({ request: r, pickup: placed(35, -85), dropoff: unplaced }, geo)).toEqual([{ lat: 35, lon: -85 }]);
	});
	it('all SAG is every placed pickup plus every positioned, checked-in vehicle', () => {
		const pts = allSagExtent(
			[
				{ request: req(), pickup: placed(35, -85), dropoff: unplaced },
				{ request: req(), pickup: unplaced, dropoff: unplaced }
			],
			geo
		);
		expect(pts).toEqual([
			{ lat: 35, lon: -85 },
			{ lat: 36, lon: -86 },
			{ lat: 37, lon: -87 }
		]);
	});
	it('a vehicle extent is its last fix, or nothing', () => {
		expect(vehicleExtent(geo[0])).toEqual([{ lat: 36, lon: -86 }]);
		expect(vehicleExtent(geo[2])).toEqual([]);
	});
	it('pads a single point to a box so a fit does not slam to max zoom', () => {
		const out = padSinglePoint([{ lat: 36, lon: -86 }]);
		expect(out).toHaveLength(2);
		expect(out[0].lat).toBeLessThan(36);
		expect(out[1].lat).toBeGreaterThan(36);
		const two = [{ lat: 1, lon: 1 }, { lat: 2, lon: 2 }];
		expect(padSinglePoint(two)).toBe(two);
		expect(padSinglePoint([])).toEqual([]);
	});
});

describe('load draft — the common answer is one confirm', () => {
	const r = req({
		slots: [
			slot({ id: 's1', bib: '334', legId: 'L1' }),
			slot({ id: 's2', riderName: 'Ann', bike: 'none', hasBike: false, legId: 'L1' }),
			slot({ id: 's3', bib: '9', legId: 'L2' })
		],
		legs: [leg({ slotIds: ['s1', 's2'], status: 'onscene' })]
	});

	it('prefills every waiting rider on the leg as aboard with their bike', () => {
		const d = loadDraft(r, r.legs[0]);
		expect(d).toEqual([
			{ slotId: 's1', name: 'Bib 334', state: 'aboard', bike: 'with_rider' },
			{ slotId: 's2', name: 'Ann', state: 'aboard', bike: 'none' }
		]);
		expect(loadSummary(d)).toBe('2 riders · 1 bike on the rack');
	});

	it('plans the load call, and a separate decline for riders who refused', () => {
		const d = loadDraft(r, r.legs[0]);
		d[1].state = 'declined';
		expect(loadPlan(d)).toEqual({ slotIds: ['s1'], bike: { s1: 'with_rider' }, decline: ['s2'] });
		d[1].state = 'stays';
		expect(loadPlan(d)).toEqual({ slotIds: ['s1'], bike: { s1: 'with_rider' }, decline: [] });
	});

	it('summarises an exception honestly', () => {
		const d = loadDraft(r, r.legs[0]);
		d[0].bike = 'left_behind';
		d[1].state = 'stays';
		expect(loadSummary(d)).toBe('1 rider · no bikes on the rack · 1 stays');
	});

	it('nobody aboard plans nothing', () => {
		const d = loadDraft(r, r.legs[0]).map((x) => ({ ...x, state: 'stays' as const }));
		expect(loadPlan(d).slotIds).toEqual([]);
	});
});

describe('deliver draft', () => {
	it('offers only the riders actually loaded on the leg', () => {
		const r = req({
			slots: [
				slot({ id: 's1', disposition: 'loaded', legId: 'L1' }),
				slot({ id: 's2', disposition: 'delivered', legId: 'L1' }),
				slot({ id: 's3', disposition: 'loaded', legId: 'L2' })
			],
			legs: [leg({ slotIds: ['s1', 's2'], status: 'loaded' })]
		});
		expect(deliverDraft(r, r.legs[0])).toEqual(['s1']);
	});
});

describe('slotName', () => {
	it('bib, then name, then a plain word', () => {
		expect(slotName(slot({ bib: '7' }))).toBe('Bib 7');
		expect(slotName(slot({ riderName: 'Ann' }))).toBe('Ann');
		expect(slotName(slot({}))).toBe('Rider');
	});
});
