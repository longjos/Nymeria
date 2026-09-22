<script lang="ts">
	import type { CheckpointWithPassages, ProgressElement, WxAlert } from '$lib/types';
	import CourseRail from '../CourseRail.svelte';

	let {
		checkpoints,
		elements,
		presentationMode = false,
		wxAlerts = [],
	}: {
		checkpoints: CheckpointWithPassages[];
		elements: ProgressElement[];
		presentationMode?: boolean;
		/** Active IN alerts — the dashboard keeps its own snapshot (no shared session with the main app). */
		wxAlerts?: WxAlert[];
	} = $props();
</script>

{#if checkpoints.length > 0}
	<section class="event-progress" class:presentation={presentationMode}>
		<h2 class="section-title">Event Progress</h2>

		<div class="progress-container">
			<CourseRail {checkpoints} {elements} {wxAlerts} density="panel" />
		</div>
	</section>
{/if}

<style>
	.event-progress {
		padding: var(--space-md);
	}

	.section-title {
		font-size: 0.75rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-text-muted);
		margin-bottom: var(--space-md);
	}

	.presentation .section-title {
		font-size: 0.9rem;
	}

	.progress-container {
		overflow-x: auto;
		padding-bottom: var(--space-sm);
	}
</style>
