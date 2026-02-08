<script lang="ts">
	import { page } from '$app/stores';
	import { onMount, tick } from 'svelte';
	import MessageBubble from '$lib/components/MessageBubble.svelte';
	import MessageCompose from '$lib/components/MessageCompose.svelte';
	import { conversations, loadMessages, initMessageStore } from '$lib/stores/messages';
	import { initStationStore } from '$lib/stores/stations';

	let callsign = $derived($page.params.callsign);
	let convo = $derived($conversations.get(callsign));
	let messages = $derived(convo?.messages ?? []);
	let scrollEl: HTMLDivElement;

	onMount(async () => {
		initStationStore();
		initMessageStore();
		await loadMessages(callsign);
	});

	$effect(() => {
		if (messages.length) {
			tick().then(() => {
				scrollEl?.scrollTo({ top: scrollEl.scrollHeight });
			});
		}
	});
</script>

<svelte:head>
	<title>{callsign} - Messages - Nymeria</title>
</svelte:head>

<div class="thread-page">
	<div class="thread-header">
		<a href="/messages" class="back">Messages</a>
		<span class="divider">/</span>
		<span class="callsign">{callsign}</span>
		<a href="/stations/{callsign}" class="station-link">View Station</a>
	</div>

	<div class="message-area" bind:this={scrollEl}>
		{#if messages.length === 0}
			<p class="empty">No messages yet. Send the first one below.</p>
		{:else}
			{#each messages as msg (msg.id)}
				<MessageBubble message={msg} />
			{/each}
		{/if}
	</div>

	<MessageCompose to={callsign} />
</div>

<style>
	.thread-page {
		max-width: 600px;
		display: flex;
		flex-direction: column;
		height: calc(100vh - 60px);
		margin: -1.5rem;
		margin-left: auto;
		margin-right: auto;
		padding: 0;
	}

	.thread-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--color-primary);
		background: var(--color-surface);
	}

	.back {
		font-size: 0.85rem;
		color: var(--color-text-muted);
		text-decoration: none;
	}

	.back:hover {
		color: var(--color-accent);
	}

	.divider {
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}

	.callsign {
		font-weight: 600;
		font-family: monospace;
		font-size: 1rem;
	}

	.station-link {
		margin-left: auto;
		font-size: 0.8rem;
		color: var(--color-accent);
		text-decoration: none;
	}

	.message-area {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
		display: flex;
		flex-direction: column;
	}

	.empty {
		margin: auto;
		color: var(--color-text-muted);
		font-size: 0.9rem;
	}
</style>
