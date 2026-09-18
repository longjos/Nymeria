<script>
	// Leaflet's own stylesheet is bundled from the local dependency, never a
	// CDN: it carries the box for every tooltip and popup on the map, and this
	// app is expected to run with no internet (see the tile cache). Loading it
	// from unpkg meant that offline, tooltip text rendered bare on the map
	// with no background, padding or border at all.
	import 'leaflet/dist/leaflet.css';
	import '../app.css';
	import Toast from '$lib/components/Toast.svelte';

	let { children } = $props();
</script>

{@render children()}
<Toast />

<style>
	:global(body) {
		overflow: hidden;
		height: 100vh;
		height: 100dvh;
	}
</style>
