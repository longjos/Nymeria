<script lang="ts">
	import { messagePath, beaconPath } from '$lib/stores/paths';
	import { formatPathDisplay } from '$lib/aprsPath';

	let {
		kind,
		label = 'via'
	}: {
		kind: 'message' | 'beacon';
		label?: string;
	} = $props();

	let path = $derived(kind === 'message' ? $messagePath : $beaconPath);
	let display = $derived(formatPathDisplay(path));
</script>

<span class="path-hint" title="AX.25 digipeater path — change in Settings → Station">
	<span class="path-via">{label}</span>
	<span class="path-value">{display}</span>
</span>

<style>
	.path-hint {
		display: inline-flex;
		align-items: baseline;
		gap: 0.3em;
		min-width: 0;
		font-size: 0.7rem;
		color: var(--color-text-muted);
		line-height: 1.3;
	}

	.path-via {
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-size: 0.62rem;
		flex-shrink: 0;
	}

	.path-value {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-weight: 600;
		color: var(--color-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
