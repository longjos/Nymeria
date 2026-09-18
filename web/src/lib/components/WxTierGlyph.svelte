<script lang="ts">
	import type { WxTier } from '$lib/types';
	import { tierMeta } from '$lib/wxAlertMeta';

	let { tier, size = 16, title }: { tier: WxTier; size?: number; title?: string } = $props();

	let meta = $derived(tierMeta[tier]);
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 16 16"
	role={title ? 'img' : undefined}
	aria-hidden={title ? undefined : true}
	aria-label={title}
>
	{#if title}<title>{title}</title>{/if}
	{#if meta.glyphFill}
		<path d={meta.glyph} fill="currentColor" />
	{:else}
		<path d={meta.glyph} fill="none" stroke="currentColor" stroke-width="1.5" />
	{/if}
	{#if tier === 'warning'}
		<!-- The "!" inside the filled triangle, punched out in the page background. -->
		<path d="M8 6v3M8 11h.01" stroke="var(--color-bg)" stroke-width="1.5" stroke-linecap="round" />
	{/if}
</svg>
