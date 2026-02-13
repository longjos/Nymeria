import { describe, it, expect } from 'vitest';
import { parseCommand, getModeIndicator, getAutocompleteContext } from './commandParser';
import type { ParsedCommand } from './commandParser';

// --- parseCommand ---

describe('parseCommand', () => {
	const noCheckedIn: string[] = [];
	const withCheckedIn = ['KD7BBC', 'W7XYZ', 'N0CALL-9'];

	describe('empty / whitespace input', () => {
		it('returns unknown for empty string', () => {
			expect(parseCommand('', noCheckedIn)).toEqual({ type: 'unknown', raw: '' });
		});

		it('returns unknown for whitespace-only input', () => {
			expect(parseCommand('   ', noCheckedIn)).toEqual({ type: 'unknown', raw: '' });
		});
	});

	describe('check-in (callsign only)', () => {
		it('parses bare callsign as check-in', () => {
			expect(parseCommand('KD7BBC', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: '', category: '',
			});
		});

		it('uppercases callsign', () => {
			expect(parseCommand('kd7bbc', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: '', category: '',
			});
		});

		it('handles callsign with SSID', () => {
			expect(parseCommand('N0CALL-9', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'N0CALL-9', traffic: '', category: '',
			});
		});
	});

	describe('check-in with traffic shortcuts', () => {
		it.each([
			['KD7BBC R', 'routine'],
			['KD7BBC P', 'priority'],
			['KD7BBC W', 'welfare'],
			['KD7BBC E', 'emergency'],
		])('parses "%s" as traffic=%s', (input, expectedTraffic) => {
			expect(parseCommand(input, noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: expectedTraffic, category: '',
			});
		});

		it('traffic shortcuts are case-insensitive (uppercase token)', () => {
			expect(parseCommand('KD7BBC r', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: 'routine', category: '',
			});
		});
	});

	describe('check-in with traffic full names', () => {
		it.each([
			['KD7BBC routine', 'routine'],
			['KD7BBC priority', 'priority'],
			['KD7BBC welfare', 'welfare'],
			['KD7BBC emergency', 'emergency'],
		])('parses "%s" as traffic=%s', (input, expectedTraffic) => {
			expect(parseCommand(input, noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: expectedTraffic, category: '',
			});
		});
	});

	describe('check-in with category', () => {
		it.each([
			'general', 'command', 'medical', 'sag', 'marshal', 'fixed', 'mobile', 'tactical',
		])('parses callsign + %s as category', (cat) => {
			expect(parseCommand(`KD7BBC ${cat}`, noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: '', category: cat,
			});
		});
	});

	describe('check-in with traffic + category', () => {
		it('parses traffic shortcut + category', () => {
			expect(parseCommand('KD7BBC E medical', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: 'emergency', category: 'medical',
			});
		});

		it('parses traffic name + category', () => {
			expect(parseCommand('KD7BBC routine command', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: 'routine', category: 'command',
			});
		});

		it('order-independent: category then traffic', () => {
			expect(parseCommand('KD7BBC medical E', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: 'emergency', category: 'medical',
			});
		});
	});

	describe('status change (only for checked-in operators)', () => {
		it.each([
			'assigned', 'enroute', 'onscene', 'brb', 'missing',
		])('parses checked-in callsign + %s as status change', (status) => {
			expect(parseCommand(`KD7BBC ${status}`, withCheckedIn)).toEqual({
				type: 'status', callsign: 'KD7BBC', status,
			});
		});

		it('status keyword for non-checked-in operator falls through to check-in', () => {
			// "assigned" is not a traffic shortcut or category, so traffic='' category=''
			expect(parseCommand('NEWCALL assigned', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'NEWCALL', traffic: '', category: '',
			});
		});

		it('case-insensitive status matching', () => {
			expect(parseCommand('KD7BBC ENROUTE', withCheckedIn)).toEqual({
				type: 'status', callsign: 'KD7BBC', status: 'enroute',
			});
		});

		it('callsign matching is case-insensitive', () => {
			expect(parseCommand('kd7bbc assigned', withCheckedIn)).toEqual({
				type: 'status', callsign: 'KD7BBC', status: 'assigned',
			});
		});

		it('"available" is a status for checked-in operators', () => {
			expect(parseCommand('KD7BBC available', withCheckedIn)).toEqual({
				type: 'status', callsign: 'KD7BBC', status: 'available',
			});
		});

		it('"released" is a status for checked-in operators', () => {
			expect(parseCommand('KD7BBC released', withCheckedIn)).toEqual({
				type: 'status', callsign: 'KD7BBC', status: 'released',
			});
		});
	});

	describe('checkout', () => {
		it('parses "CALL out" as checkout', () => {
			expect(parseCommand('KD7BBC out', noCheckedIn)).toEqual({
				type: 'checkout', callsign: 'KD7BBC',
			});
		});

		it('case-insensitive', () => {
			expect(parseCommand('kd7bbc OUT', noCheckedIn)).toEqual({
				type: 'checkout', callsign: 'KD7BBC',
			});
		});
	});

	describe('note', () => {
		it('parses "CALL note text" as note', () => {
			expect(parseCommand('KD7BBC note needs water', noCheckedIn)).toEqual({
				type: 'note', callsign: 'KD7BBC', text: 'needs water',
			});
		});

		it('parses "CALL n text" as note (shorthand)', () => {
			expect(parseCommand('KD7BBC n arrived at checkpoint', noCheckedIn)).toEqual({
				type: 'note', callsign: 'KD7BBC', text: 'arrived at checkpoint',
			});
		});

		it('empty note text is allowed', () => {
			expect(parseCommand('KD7BBC note', noCheckedIn)).toEqual({
				type: 'note', callsign: 'KD7BBC', text: '',
			});
		});

		it('preserves multi-word note text', () => {
			expect(parseCommand('W7XYZ note lost radio contact at mile 5', noCheckedIn)).toEqual({
				type: 'note', callsign: 'W7XYZ', text: 'lost radio contact at mile 5',
			});
		});
	});

	describe('mission create', () => {
		it('parses "mission <title>"', () => {
			expect(parseCommand('mission Search Grid Alpha', noCheckedIn)).toEqual({
				type: 'mission_create', title: 'Search Grid Alpha',
			});
		});

		it('case-insensitive "mission" keyword', () => {
			expect(parseCommand('MISSION Water Resupply', noCheckedIn)).toEqual({
				type: 'mission_create', title: 'Water Resupply',
			});
		});

		it('mission with no title returns unknown', () => {
			expect(parseCommand('mission', noCheckedIn)).toEqual({
				type: 'unknown', raw: 'mission',
			});
		});

		it('mission with only spaces after returns unknown', () => {
			expect(parseCommand('mission   ', noCheckedIn)).toEqual({
				type: 'unknown', raw: 'mission',
			});
		});
	});

	describe('mission assign', () => {
		it('parses "CALL assign <mission>"', () => {
			expect(parseCommand('KD7BBC assign Search Grid Alpha', noCheckedIn)).toEqual({
				type: 'mission_assign', callsign: 'KD7BBC', missionTitle: 'Search Grid Alpha',
			});
		});

		it('empty mission title is allowed (for autocomplete)', () => {
			expect(parseCommand('KD7BBC assign', noCheckedIn)).toEqual({
				type: 'mission_assign', callsign: 'KD7BBC', missionTitle: '',
			});
		});
	});

	describe('location', () => {
		it('parses "CALL loc <lat> <lon>"', () => {
			expect(parseCommand('KD7BBC loc 45.523 -122.676', noCheckedIn)).toEqual({
				type: 'location', callsign: 'KD7BBC', lat: 45.523, lon: -122.676,
			});
		});

		it('parses "CALL loc <name>"', () => {
			expect(parseCommand('KD7BBC loc Aid Station 3', noCheckedIn)).toEqual({
				type: 'location', callsign: 'KD7BBC', locationName: 'Aid Station 3',
			});
		});

		it('parses "CALL loc" with no arguments', () => {
			expect(parseCommand('KD7BBC loc', noCheckedIn)).toEqual({
				type: 'location', callsign: 'KD7BBC',
			});
		});

		it('does not confuse partial numbers as coordinates', () => {
			// Only one number — treat as location name
			expect(parseCommand('KD7BBC loc 45.5', noCheckedIn)).toEqual({
				type: 'location', callsign: 'KD7BBC', locationName: '45.5',
			});
		});

		it('non-numeric after loc treated as name', () => {
			expect(parseCommand('KD7BBC loc HQ', noCheckedIn)).toEqual({
				type: 'location', callsign: 'KD7BBC', locationName: 'HQ',
			});
		});
	});

	describe('priority: action keywords beat status for checked-in operators', () => {
		it('"out" always means checkout, even for checked-in', () => {
			expect(parseCommand('KD7BBC out', withCheckedIn)).toEqual({
				type: 'checkout', callsign: 'KD7BBC',
			});
		});

		it('"note" always means note, even for checked-in', () => {
			expect(parseCommand('KD7BBC note test', withCheckedIn)).toEqual({
				type: 'note', callsign: 'KD7BBC', text: 'test',
			});
		});

		it('"assign" always means mission assign', () => {
			expect(parseCommand('KD7BBC assign Water Run', withCheckedIn)).toEqual({
				type: 'mission_assign', callsign: 'KD7BBC', missionTitle: 'Water Run',
			});
		});

		it('"loc" always means location', () => {
			expect(parseCommand('KD7BBC loc HQ', withCheckedIn)).toEqual({
				type: 'location', callsign: 'KD7BBC', locationName: 'HQ',
			});
		});
	});

	describe('extra whitespace handling', () => {
		it('trims leading/trailing whitespace', () => {
			expect(parseCommand('  KD7BBC  R  ', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: 'routine', category: '',
			});
		});

		it('handles multiple spaces between tokens', () => {
			expect(parseCommand('KD7BBC    E    medical', noCheckedIn)).toEqual({
				type: 'checkin', callsign: 'KD7BBC', traffic: 'emergency', category: 'medical',
			});
		});
	});

	describe('SSID callsigns in checked-in list', () => {
		it('recognizes SSID callsign for status change', () => {
			expect(parseCommand('N0CALL-9 assigned', withCheckedIn)).toEqual({
				type: 'status', callsign: 'N0CALL-9', status: 'assigned',
			});
		});
	});
});

// --- getModeIndicator ---

describe('getModeIndicator', () => {
	it('check-in with no details', () => {
		expect(getModeIndicator({ type: 'checkin', callsign: 'X', traffic: '', category: '' }))
			.toBe('[CHECK IN]');
	});

	it('check-in with traffic', () => {
		expect(getModeIndicator({ type: 'checkin', callsign: 'X', traffic: 'routine', category: '' }))
			.toBe('[CHECK IN · routine]');
	});

	it('check-in with traffic + category', () => {
		expect(getModeIndicator({ type: 'checkin', callsign: 'X', traffic: 'emergency', category: 'medical' }))
			.toBe('[CHECK IN · emergency · medical]');
	});

	it('check-in with category only', () => {
		expect(getModeIndicator({ type: 'checkin', callsign: 'X', traffic: '', category: 'command' }))
			.toBe('[CHECK IN · command]');
	});

	it('status change', () => {
		expect(getModeIndicator({ type: 'status', callsign: 'X', status: 'assigned' }))
			.toBe('[STATUS → assigned]');
	});

	it('checkout', () => {
		expect(getModeIndicator({ type: 'checkout', callsign: 'X' }))
			.toBe('[CHECKOUT]');
	});

	it('note', () => {
		expect(getModeIndicator({ type: 'note', callsign: 'X', text: 'foo' }))
			.toBe('[NOTE]');
	});

	it('mission create', () => {
		expect(getModeIndicator({ type: 'mission_create', title: 'Foo' }))
			.toBe('[NEW MISSION]');
	});

	it('mission assign', () => {
		expect(getModeIndicator({ type: 'mission_assign', callsign: 'X', missionTitle: 'Foo' }))
			.toBe('[ASSIGN MISSION]');
	});

	it('location', () => {
		expect(getModeIndicator({ type: 'location', callsign: 'X' }))
			.toBe('[LOCATION]');
	});

	it('unknown returns empty string', () => {
		expect(getModeIndicator({ type: 'unknown', raw: 'x' }))
			.toBe('');
	});
});

// --- getAutocompleteContext ---

describe('getAutocompleteContext', () => {
	const checkedIn = ['KD7BBC', 'W7XYZ', 'N0CALL-9'];
	const missions = ['Search Grid Alpha', 'Water Resupply', 'Checkpoint Bravo'];

	it('returns null for empty input', () => {
		expect(getAutocompleteContext('', checkedIn, missions)).toBeNull();
	});

	it('returns null for whitespace-only input', () => {
		expect(getAutocompleteContext('   ', checkedIn, missions)).toBeNull();
	});

	describe('first token (callsign phase)', () => {
		it('suggests matching callsigns', () => {
			const ctx = getAutocompleteContext('KD', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('callsign');
			expect(ctx!.suggestions).toContain('KD7BBC');
			expect(ctx!.suggestions).not.toContain('W7XYZ');
		});

		it('suggests "mission" when typing "m"', () => {
			const ctx = getAutocompleteContext('m', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.suggestions).toContain('mission');
		});

		it('suggests "mission" when typing "mis"', () => {
			const ctx = getAutocompleteContext('mis', checkedIn, missions);
			expect(ctx!.suggestions).toContain('mission');
		});
	});

	describe('after callsign + space (action phase)', () => {
		it('suggests actions after callsign space', () => {
			const ctx = getAutocompleteContext('KD7BBC ', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('action');
			expect(ctx!.suggestions).toContain('out');
			expect(ctx!.suggestions).toContain('note');
			expect(ctx!.suggestions).toContain('assign');
			expect(ctx!.suggestions).toContain('loc');
			// Traffic shortcuts
			expect(ctx!.suggestions).toContain('R');
			expect(ctx!.suggestions).toContain('E');
		});

		it('includes status suggestions for checked-in operators', () => {
			const ctx = getAutocompleteContext('KD7BBC ', checkedIn, missions);
			expect(ctx!.suggestions).toContain('assigned');
			expect(ctx!.suggestions).toContain('enroute');
			expect(ctx!.suggestions).toContain('missing');
		});

		it('excludes status suggestions for non-checked-in operators', () => {
			const ctx = getAutocompleteContext('NEWCALL ', checkedIn, missions);
			expect(ctx!.suggestions).not.toContain('assigned');
			expect(ctx!.suggestions).not.toContain('enroute');
		});
	});

	describe('partial second token', () => {
		it('filters action keywords by partial', () => {
			const ctx = getAutocompleteContext('KD7BBC as', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('action');
			expect(ctx!.suggestions).toContain('assign');
			expect(ctx!.suggestions).toContain('assigned');
			expect(ctx!.suggestions).not.toContain('out');
		});

		it('filters traffic names by partial', () => {
			const ctx = getAutocompleteContext('KD7BBC ro', checkedIn, missions);
			expect(ctx!.suggestions).toContain('routine');
		});

		it('filters categories by partial', () => {
			const ctx = getAutocompleteContext('KD7BBC med', checkedIn, missions);
			expect(ctx!.suggestions).toContain('medical');
		});
	});

	describe('mission assign autocomplete', () => {
		it('suggests mission titles after "assign "', () => {
			const ctx = getAutocompleteContext('KD7BBC assign ', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('mission_name');
			expect(ctx!.suggestions).toEqual(missions);
		});

		it('filters mission titles by partial', () => {
			const ctx = getAutocompleteContext('KD7BBC assign search', checkedIn, missions);
			expect(ctx!.suggestions).toContain('Search Grid Alpha');
			expect(ctx!.suggestions).not.toContain('Water Resupply');
		});

		it('finds mission title by substring match', () => {
			const ctx = getAutocompleteContext('KD7BBC assign bravo', checkedIn, missions);
			expect(ctx!.suggestions).toContain('Checkpoint Bravo');
		});
	});

	describe('note autocomplete', () => {
		it('returns note_text phase with no suggestions', () => {
			const ctx = getAutocompleteContext('KD7BBC note some text', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('note_text');
			expect(ctx!.suggestions).toEqual([]);
		});

		it('works with shorthand "n"', () => {
			const ctx = getAutocompleteContext('KD7BBC n typing', checkedIn, missions);
			expect(ctx!.phase).toBe('note_text');
		});
	});

	describe('location autocomplete', () => {
		it('returns location phase after "loc "', () => {
			const ctx = getAutocompleteContext('KD7BBC loc ', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('location');
		});
	});

	describe('mission title autocomplete', () => {
		it('returns mission_title phase after "mission "', () => {
			const ctx = getAutocompleteContext('mission ', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('mission_title');
			expect(ctx!.suggestions).toEqual([]);
		});
	});

	describe('third+ token: category suggestions', () => {
		it('suggests categories after traffic token + space', () => {
			const ctx = getAutocompleteContext('KD7BBC R ', checkedIn, missions);
			expect(ctx).not.toBeNull();
			expect(ctx!.phase).toBe('category');
			expect(ctx!.suggestions).toContain('medical');
			expect(ctx!.suggestions).toContain('command');
		});

		it('filters categories by partial third token', () => {
			const ctx = getAutocompleteContext('KD7BBC R me', checkedIn, missions);
			expect(ctx!.suggestions).toContain('medical');
			expect(ctx!.suggestions).not.toContain('command');
		});
	});
});
