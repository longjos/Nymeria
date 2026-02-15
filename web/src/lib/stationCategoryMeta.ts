import type { StationCategory } from './types';

export const stationCategoryMeta: Record<StationCategory, { label: string; short: string; color: string }> = {
	general:  { label: 'General',  short: 'GEN', color: '#6b7280' },
	command:  { label: 'Command',  short: 'CMD', color: '#eab308' },
	medical:  { label: 'Medical',  short: 'MED', color: '#ef4444' },
	sag:      { label: 'SAG',      short: 'SAG', color: '#f97316' },
	marshal:  { label: 'Marshal',  short: 'MAR', color: '#3b82f6' },
	fixed:    { label: 'Fixed',    short: 'FIX', color: '#14b8a6' },
	mobile:   { label: 'Mobile',   short: 'MOB', color: '#8b5cf6' },
	tactical: { label: 'Tactical', short: 'TAC', color: '#6366f1' },
};
