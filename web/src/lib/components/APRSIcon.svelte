<script lang="ts">
	import type { APRSSymbol } from '$lib/types';
	import { symbolInfo, symbolChar } from '$lib/symbols';

	let { symbol, size = 24 }: { symbol: APRSSymbol; size?: number } = $props();

	let info = $derived(symbolInfo(symbol));
	let char = $derived(symbolChar(symbol));
</script>

<span
	class="aprs-icon"
	style="
		width: {size}px;
		height: {size}px;
		font-size: {size * 0.55}px;
		background: {info.color};
	"
	title={info.label}
>
	{char}
</span>

<style>
	.aprs-icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		color: white;
		font-weight: 700;
		font-family: monospace;
		flex-shrink: 0;
		line-height: 1;
	}
</style>
