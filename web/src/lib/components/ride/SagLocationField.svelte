<script lang="ts">
	// Shared editor for a store.SAGLocation — used for both a SAG request's
	// pickup and dropoff (each with its own allowed `kinds`, per GET
	// /nets/{id}/sag/config's pickupKinds/dropoffKinds; never hardcode which
	// subset applies here). Mile marker is the primary field, a coordinate is
	// secondary/collapsed — the composer brief's exact ordering.
	import type { SAGLocation, Annotation } from '$lib/types';
	import { SAG_LOCATION_KIND_LABELS, SAG_LOCATION_KIND_ANNOTATION_CATEGORY } from '$lib/rideMeta';

	let {
		value,
		kinds,
		annotations,
		legend,
		idPrefix,
		onChange
	}: {
		value: SAGLocation;
		kinds: string[];
		annotations: Annotation[];
		legend: string;
		idPrefix: string;
		onChange: (v: SAGLocation) => void;
	} = $props();

	let showCoords = $state(!!(value.lat != null || value.lon != null));

	function set(patch: Partial<SAGLocation>): void {
		onChange({ ...value, ...patch });
	}

	function numOrUndef(raw: string): number | undefined {
		if (raw.trim() === '') return undefined;
		const n = Number(raw);
		return Number.isNaN(n) ? undefined : n;
	}

	let annotationCategory = $derived(SAG_LOCATION_KIND_ANNOTATION_CATEGORY[value.kind]);
	let matchingAnnotations = $derived(annotationCategory ? annotations.filter((a) => a.category === annotationCategory) : []);

	function pickAnnotation(id: string): void {
		if (!id) {
			set({ annotationId: '' });
			return;
		}
		const ann = annotations.find((a) => a.id === id);
		if (!ann) return;
		let lat: number | undefined;
		let lon: number | undefined;
		try {
			const geo = JSON.parse(ann.geometry);
			if (geo?.type === 'Point' && Array.isArray(geo.coordinates)) {
				lon = geo.coordinates[0];
				lat = geo.coordinates[1];
			}
		} catch {
			// unparsable geometry — annotation link still carries the name
		}
		set({ annotationId: ann.id, description: ann.label, lat, lon });
		showCoords = lat != null;
	}
</script>

<fieldset class="slf">
	<legend class="slf-legend">{legend}</legend>

	<label class="slf-field" for="{idPrefix}-kind">
		<span class="slf-label">Kind</span>
		<select
			id="{idPrefix}-kind"
			value={value.kind}
			onchange={(e) => set({ kind: (e.target as HTMLSelectElement).value, annotationId: '' })}
		>
			{#each kinds as k (k)}
				<option value={k}>{SAG_LOCATION_KIND_LABELS[k] ?? k}</option>
			{/each}
		</select>
	</label>

	{#if matchingAnnotations.length > 0}
		<label class="slf-field" for="{idPrefix}-ann">
			<span class="slf-label">Which one</span>
			<select id="{idPrefix}-ann" value={value.annotationId ?? ''} onchange={(e) => pickAnnotation((e.target as HTMLSelectElement).value)}>
				<option value="">Choose…</option>
				{#each matchingAnnotations as a (a.id)}
					<option value={a.id}>{a.label}</option>
				{/each}
			</select>
		</label>
	{/if}

	<div class="slf-row">
		<label class="slf-field" for="{idPrefix}-mile">
			<span class="slf-label">Mile marker</span>
			<input
				id="{idPrefix}-mile"
				type="number"
				step="0.1"
				inputmode="decimal"
				placeholder="31.4"
				value={value.mileMarker ?? ''}
				oninput={(e) => set({ mileMarker: numOrUndef((e.target as HTMLInputElement).value) })}
			/>
		</label>
		<label class="slf-field" for="{idPrefix}-remain">
			<span class="slf-label">Miles remaining (on-air)</span>
			<input
				id="{idPrefix}-remain"
				type="number"
				step="0.1"
				inputmode="decimal"
				placeholder="31 to go"
				value={value.milesRemaining ?? ''}
				oninput={(e) => set({ milesRemaining: numOrUndef((e.target as HTMLInputElement).value) })}
			/>
		</label>
	</div>

	<label class="slf-field" for="{idPrefix}-desc">
		<span class="slf-label">Description</span>
		<input
			id="{idPrefix}-desc"
			type="text"
			placeholder="just past the cattle guard on Skyline"
			value={value.description ?? ''}
			oninput={(e) => set({ description: (e.target as HTMLInputElement).value })}
		/>
	</label>

	<button type="button" class="slf-coord-toggle" onclick={() => (showCoords = !showCoords)} aria-expanded={showCoords}>
		{showCoords ? '− Hide' : '+ Add'} coordinates
	</button>
	{#if showCoords}
		<div class="slf-row">
			<label class="slf-field" for="{idPrefix}-lat">
				<span class="slf-label">Lat</span>
				<input
					id="{idPrefix}-lat"
					type="number"
					step="0.00001"
					value={value.lat ?? ''}
					oninput={(e) => set({ lat: numOrUndef((e.target as HTMLInputElement).value) })}
				/>
			</label>
			<label class="slf-field" for="{idPrefix}-lon">
				<span class="slf-label">Lon</span>
				<input
					id="{idPrefix}-lon"
					type="number"
					step="0.00001"
					value={value.lon ?? ''}
					oninput={(e) => set({ lon: numOrUndef((e.target as HTMLInputElement).value) })}
				/>
			</label>
		</div>
	{/if}
</fieldset>

<style>
	.slf {
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		padding: var(--space-sm);
		display: flex;
		flex-direction: column;
		gap: var(--space-sm);
		margin: 0;
	}

	.slf-legend {
		font-size: var(--ride-t-label);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
		color: var(--color-text-muted);
		padding: 0 var(--space-xs);
	}

	.slf-row {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-sm);
	}

	.slf-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-2xs);
		font-size: var(--ride-t-body);
	}

	.slf-label {
		font-size: var(--ride-t-label);
		color: var(--color-text-muted);
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: var(--ride-label-tracking);
	}

	.slf-field input,
	.slf-field select {
		min-height: 36px;
		padding: 0 var(--space-sm);
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font: inherit;
	}

	.slf-coord-toggle {
		align-self: flex-start;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: var(--ride-t-label);
		cursor: pointer;
		padding: var(--space-xs) 0;
		min-height: 32px;
	}
</style>
