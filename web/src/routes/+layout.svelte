<script>
	import '../app.css';
	import Toast from '$lib/components/Toast.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<link
		rel="stylesheet"
		href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css"
		crossorigin=""
	/>
</svelte:head>

{@render children()}
<Toast />

<style>
	:global(body) {
		overflow: hidden;
		height: 100vh;
		height: 100dvh;
	}
</style>
