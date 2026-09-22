<script lang="ts">
	// Generalized from WxInterruptBanner.svelte (spec §0/§4/§12): tier + queue
	// + ack + focus-move + chime + vibrate, source-agnostic. WxInterruptBanner
	// is now a thin wrapper that builds an InterruptItem from the wx stores;
	// RideInterruptBanner does the same from the ride stores. Never build a
	// second full-width interrupt banner — this is the ONE.
	import { fly } from 'svelte/transition';
	import type { InterruptItem } from '$lib/stores/interrupts';
	import { lastAudibleAt, SOUND_BUDGET_MS } from '$lib/stores/interrupts';
	import WxTierGlyph from './WxTierGlyph.svelte';
	import RideTierGlyph from './RideTierGlyph.svelte';

	let {
		item,
		more,
		sounds,
		onAck,
		onRotate
	}: {
		item: InterruptItem | null;
		more: number;
		sounds: boolean;
		onAck: () => Promise<void>;
		onRotate: () => void;
	} = $props();

	let el: HTMLElement | undefined = $state();
	let ackBtn: HTMLButtonElement | undefined = $state();

	let audioCtx: AudioContext | null = null;

	/** Three 200 ms 880 Hz square-wave pulses, 100 ms apart — created lazily on first use per autoplay policy. */
	function playChime(): void {
		try {
			if (!audioCtx) {
				const Ctor = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
				if (!Ctor) return;
				audioCtx = new Ctor();
			}
			if (audioCtx.state === 'suspended') return; // no user gesture yet — skip silently
			const now = audioCtx.currentTime;
			for (let i = 0; i < 3; i++) {
				const osc = audioCtx.createOscillator();
				const gain = audioCtx.createGain();
				osc.type = 'square';
				osc.frequency.value = 880;
				gain.gain.value = 0.2;
				osc.connect(gain).connect(audioCtx.destination);
				const start = now + i * 0.3;
				osc.start(start);
				osc.stop(start + 0.2);
			}
		} catch {
			// Web Audio unavailable — skip silently, the visual banner still appears.
		}
	}

	/** Sound budget (spec §4.7): at most one audible event per 30s app-wide, EMERGENCY always wins arbitration by virtue of owning the queue upstream. */
	function playSignal(): void {
		if (!sounds) return;
		const now = Date.now();
		let last = 0;
		lastAudibleAt.subscribe((v) => (last = v))();
		if (now - last < SOUND_BUDGET_MS) return;
		lastAudibleAt.set(now);
		try {
			navigator.vibrate?.([200, 100, 200]);
		} catch {
			// Vibration API unavailable — skip silently.
		}
		playChime();
	}

	let lastHeadId: string | null = null;
	let repeatTimer: ReturnType<typeof setInterval> | null = null;

	function clearRepeat() {
		if (repeatTimer) {
			clearInterval(repeatTimer);
			repeatTimer = null;
		}
	}

	// Fires the signal and moves focus to Acknowledge whenever the head of the
	// queue changes identity (a new interrupt arrives, or the previous one is
	// dismissed and the next takes over). Also arms the re-chime for items
	// that carry repeatEveryMs (ride EMERGENCY: 60s until acknowledged).
	$effect(() => {
		const id = item?.id ?? null;
		if (id && id !== lastHeadId) {
			lastHeadId = id;
			playSignal();
			queueMicrotask(() => ackBtn?.focus());
			clearRepeat();
			if (item?.repeatEveryMs) {
				repeatTimer = setInterval(() => playSignal(), item.repeatEveryMs);
			}
		} else if (!id) {
			lastHeadId = null;
			clearRepeat();
		}
		return () => clearRepeat();
	});

	function handleWindowKeydown(e: KeyboardEvent): void {
		if (!item || e.key !== 'Escape') return;
		// Esc never dismisses the banner — it only returns focus to the page.
		e.preventDefault();
		(document.activeElement as HTMLElement | null)?.blur();
	}

	function trapTab(e: KeyboardEvent): void {
		if (e.key !== 'Tab' || !el) return;
		const focusables = Array.from(el.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'));
		if (focusables.length === 0) return;
		const first = focusables[0];
		const last = focusables[focusables.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	async function handleAck(): Promise<void> {
		clearRepeat();
		await onAck();
	}

	async function handleAckForNet(): Promise<void> {
		if (item?.ackForNet) await item.ackForNet();
	}
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if item}
	<div
		class="ib-banner"
		role="alertdialog"
		tabindex="-1"
		aria-modal="true"
		data-blocks-escape="true"
		aria-labelledby="ib-title"
		aria-describedby="ib-desc"
		style="border-color: var({item.borderVar})"
		bind:this={el}
		onkeydown={trapTab}
		transition:fly={{ y: 24, duration: 250 }}
	>
		<div class="ib-band" class:ib-hatched={item.hatched} style="background: var({item.borderVar})" aria-hidden="true"></div>
		<div class="ib-head">
			{#if item.glyph.kind === 'wx'}
				<WxTierGlyph tier={item.glyph.tier} size={22} />
			{:else}
				<RideTierGlyph tier={item.glyph.tier} size={22} />
			{/if}
			<h2 id="ib-title" class="ib-title">{item.title}</h2>
			<span class="ib-sub">{item.sub}</span>
		</div>
		<p id="ib-desc" class="ib-body">{item.body}</p>
		{#if item.where}<p class="ib-where">{item.where}</p>{/if}
		{#if item.endsOrAt}<p class="ib-ends">{item.endsOrAt}</p>{/if}
		{#if item.instruction}<p class="ib-instr">{item.instruction}</p>{/if}
		<button
			class="ib-ack"
			class:ib-ack-dark={item.ackTextDark}
			style="background: var({item.borderVar})"
			bind:this={ackBtn}
			onclick={handleAck}
		>
			ACKNOWLEDGE
		</button>
		<div class="ib-row">
			{#if item.showOnMap}<button class="ib-btn" onclick={item.showOnMap}>Show on map</button>{/if}
			{#if item.details}<button class="ib-btn" onclick={item.details}>Details</button>{/if}
			{#if item.ackForNet}<button class="ib-btn" onclick={handleAckForNet}>Ack for net</button>{/if}
		</div>
		<footer class="ib-foot">
			<span>{item.footer}</span>
			{#if more > 0}
				<button class="ib-more" onclick={onRotate}>+{more} more ▸</button>
			{/if}
		</footer>
	</div>
{/if}

<style>
	.ib-banner {
		position: fixed;
		z-index: var(--z-overlay);
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
		overflow: hidden;
		border: 1px solid;
	}

	@media (min-width: 769px) {
		.ib-banner {
			right: var(--space-lg);
			bottom: calc(var(--space-lg) + 56px + var(--ride-strip-h, 0px));
			max-width: 480px;
			width: calc(100vw - 2 * var(--space-lg));
		}
	}

	@media (max-width: 768px) {
		.ib-banner {
			left: var(--space-md);
			right: var(--space-md);
			bottom: calc(var(--sheet-peek) + var(--space-md) + env(safe-area-inset-bottom));
		}
	}

	.ib-band {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		height: 6px;
	}

	.ib-hatched {
		background-image: repeating-linear-gradient(45deg, rgba(255, 255, 255, 0.35) 0, rgba(255, 255, 255, 0.35) 2px, transparent 2px, transparent 6px);
	}

	.ib-head {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	.ib-title {
		flex: 1;
		min-width: 0;
		font-size: 1.05rem;
		font-weight: 700;
	}

	.ib-sub {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.ib-body {
		font-size: 0.85rem;
		line-height: 1.4;
	}

	.ib-where,
	.ib-ends {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.ib-instr {
		font-size: 0.8rem;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.ib-ack {
		min-height: 56px;
		width: 100%;
		margin-top: var(--space-xs);
		border: none;
		border-radius: var(--radius-md);
		color: white;
		font-size: 1.05rem;
		font-weight: 800;
		letter-spacing: 0.04em;
		cursor: pointer;
	}

	.ib-ack-dark {
		color: var(--color-on-warning);
	}

	.ib-row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
	}

	.ib-btn {
		flex: 1;
		min-height: 44px;
		min-width: 100px;
		background: var(--color-bg);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.ib-btn:hover,
	.ib-btn:focus-visible {
		border-color: var(--color-accent);
	}

	.ib-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.ib-more {
		flex-shrink: 0;
		background: none;
		border: none;
		padding: 0;
		color: var(--color-text-muted);
		font-size: 0.7rem;
		font-weight: 600;
		cursor: pointer;
		min-height: 32px;
	}
</style>
