import type { AnnotationCategory } from '$lib/types';

export interface LocationTemplate {
	name: string;
	shortName: string;
	category: AnnotationCategory;
	description: string;
}

export interface EventTemplate {
	name: string;
	description: string;
	locations: LocationTemplate[];
}

export const eventTemplates: EventTemplate[] = [
	{
		name: 'Bike Event',
		description: 'Typical century ride or bike race with aid stations and SAG support',
		locations: [
			{ name: 'Start Line', shortName: 'START', category: 'start', description: 'Event start/registration area' },
			{ name: 'Finish Line', shortName: 'FINISH', category: 'finish', description: 'Event finish area' },
			{ name: 'Aid Station 1', shortName: 'AID-1', category: 'aid', description: 'First rest stop with water and supplies' },
			{ name: 'Aid Station 2', shortName: 'AID-2', category: 'aid', description: 'Second rest stop with water and supplies' },
			{ name: 'SAG Vehicle', shortName: 'SAG', category: 'general', description: 'Support and gear vehicle for rider assistance' },
			{ name: 'Net Control', shortName: 'NCS', category: 'staging', description: 'Net control station location' },
		]
	},
	{
		name: 'Parade',
		description: 'Parade or march event with staging, checkpoints, and medical',
		locations: [
			{ name: 'Staging Area', shortName: 'STAGE', category: 'staging', description: 'Pre-event staging and lineup area' },
			{ name: 'Parade Start', shortName: 'START', category: 'start', description: 'Parade departure point' },
			{ name: 'Parade End', shortName: 'END', category: 'finish', description: 'Parade terminus/dispersal area' },
			{ name: 'Medical', shortName: 'MED', category: 'aid', description: 'First aid / medical station' },
			{ name: 'Checkpoint 1', shortName: 'CP-1', category: 'checkpoint', description: 'Route observation checkpoint' },
			{ name: 'Checkpoint 2', shortName: 'CP-2', category: 'checkpoint', description: 'Route observation checkpoint' },
		]
	},
	{
		name: 'Shelter Ops',
		description: 'Emergency shelter operation with supply depot and EOC',
		locations: [
			{ name: 'Shelter A', shortName: 'SHL-A', category: 'shelter', description: 'Primary shelter facility' },
			{ name: 'Shelter B', shortName: 'SHL-B', category: 'shelter', description: 'Secondary shelter facility' },
			{ name: 'Supply Depot', shortName: 'SUPPLY', category: 'staging', description: 'Central supply distribution point' },
			{ name: 'EOC', shortName: 'EOC', category: 'staging', description: 'Emergency operations center' },
			{ name: 'Evacuation Point', shortName: 'EVAC', category: 'hazard', description: 'Evacuation staging/pickup point' },
		]
	}
];
