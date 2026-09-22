<script lang="ts">
	// Near-copy of WxTierGlyph.svelte for ride priority tiers. Style is keyed
	// by RANK POSITION (see rideMeta.ts tierStyle), never by tier id/label —
	// the five names are agency-configurable data.
	import type { PriorityTier } from '$lib/types';
	import { tierStyle } from '$lib/rideMeta';

	let { tier, size = 16, title }: { tier: PriorityTier; size?: number; title?: string } = $props();

	let meta = $derived(tierStyle(tier));
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 16 16"
	role={title ? 'img' : undefined}
	aria-hidden={title ? undefined : true}
	aria-label={title}
	style="color: var({meta.colorVar})"
>
	{#if title}<title>{title}</title>{/if}
	{#if meta.glyphFill}
		<path d={meta.glyph} fill="currentColor" />
	{:else}
		<path d={meta.glyph} fill="none" stroke="currentColor" stroke-width="1.5" />
	{/if}
	{#if meta.punch}
		<!-- The "!" punched out in the page background, exactly as WxTierGlyph does for its warning tier. -->
		<path d="M8 6v3M8 11h.01" stroke="var(--color-bg)" stroke-width="1.5" stroke-linecap="round" />
	{/if}
</svg>
