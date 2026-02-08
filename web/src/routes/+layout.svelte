<script>
	import '../app.css';
	import { page } from '$app/stores';

	let { children } = $props();

	let currentPath = $derived($page.url.pathname);
</script>

<header>
	<nav>
		<a href="/" class="logo">Nymeria</a>
		<div class="nav-links">
			<a href="/map" class:active={currentPath.startsWith('/map')}>Map</a>
			<a href="/stations" class:active={currentPath.startsWith('/stations')}>Stations</a>
			<a href="/messages" class:active={currentPath.startsWith('/messages')}>Messages</a>
		</div>
	</nav>
</header>

<main>
	{@render children()}
</main>

<style>
	header {
		background: var(--color-surface);
		padding: 0 1.5rem;
		border-bottom: 1px solid var(--color-primary);
		height: 52px;
		display: flex;
		align-items: center;
	}

	nav {
		display: flex;
		align-items: center;
		gap: 2rem;
		width: 100%;
		max-width: 1200px;
		margin: 0 auto;
	}

	.logo {
		font-size: 1.1rem;
		font-weight: 700;
		text-decoration: none;
		color: var(--color-accent);
		letter-spacing: 0.02em;
	}

	.nav-links {
		display: flex;
		gap: 0.25rem;
	}

	.nav-links a {
		padding: 0.375rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 500;
		text-decoration: none;
		color: var(--color-text-muted);
		transition: color 0.15s, background 0.15s;
	}

	.nav-links a:hover {
		color: var(--color-text);
		background: var(--color-primary);
	}

	.nav-links a.active {
		color: var(--color-text);
		background: var(--color-primary);
	}

	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
