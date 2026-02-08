<script>
	import '../app.css';

	let { children } = $props();
</script>

<header>
	<nav>
		<a href="/" class="logo">Nymeria</a>
		<span class="tagline">APRS Client</span>
	</nav>
</header>

<main>
	{@render children()}
</main>

<style>
	header {
		background: var(--color-surface);
		padding: 0.75rem 1.5rem;
		border-bottom: 1px solid var(--color-primary);
	}

	nav {
		display: flex;
		align-items: center;
		gap: 1rem;
		max-width: 1200px;
		margin: 0 auto;
	}

	.logo {
		font-size: 1.25rem;
		font-weight: 700;
		text-decoration: none;
		color: var(--color-accent);
	}

	.tagline {
		color: var(--color-text-muted);
		font-size: 0.875rem;
	}

	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}
</style>
