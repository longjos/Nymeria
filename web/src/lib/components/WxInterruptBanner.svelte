<script lang="ts">
	import { fly } from 'svelte/transition';
	import {
		wxActiveInterrupt,
		wxInterruptMore,
		wxInterruptQueue,
		wxIsNcs,
		wxLinkStatus,
		wxClock,
		ackAlert,
		ackAlertForNet,
		showAlertOnMap,
		openWxAlert
	} from '$lib/stores/wxAlerts';
	import { tierMeta, announceSentence } from '$lib/wxAlertMeta';
	import { clock, clockWithSeconds, countdown } from '$lib/wxAlertTime';
	import WxTierGlyph from './WxTierGlyph.svelte';

	let el: HTMLElement | undefined = $state();
	let ackBtn: HTMLButtonElement | undefined = $state();

	let alert = $derived($wxActiveInterrupt);
	let meta = $derived(alert ? tierMeta[alert.tier] : null);
	let cd = $derived(alert ? countdown(alert.endsAt, $wxClock) : null);
	let body = $derived(alert ? announceSentence(alert, clockWithSeconds(alert.fetchedAt), $wxClock) : '');

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

	function playSignal(): void {
		if (!$wxLinkStatus.sounds) return;
		try {
			navigator.vibrate?.([200, 100, 200]);
		} catch {
			// Vibration API unavailable — skip silently.
		}
		playChime();
	}

	let lastHeadId: string | null = null;

	// Fires the signal and moves focus to Acknowledge whenever the head of the
	// queue changes identity (a new interrupt arrives, or the previous one is
	// dismissed and the next takes over).
	$effect(() => {
		const id = alert?.id ?? null;
		if (id && id !== lastHeadId) {
			lastHeadId = id;
			playSignal();
			queueMicrotask(() => ackBtn?.focus());
		} else if (!id) {
			lastHeadId = null;
		}
	});

	function handleWindowKeydown(e: KeyboardEvent): void {
		if (!alert || e.key !== 'Escape') return;
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
		if (alert) await ackAlert(alert.id);
	}

	async function handleAckForNet(): Promise<void> {
		if (alert) await ackAlertForNet(alert.id);
	}

	function rotateQueue(): void {
		wxInterruptQueue.update((q) => (q.length > 1 ? [...q.slice(1), q[0]] : q));
	}
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if alert && meta && cd}
	<div
		class="wx-banner wx-banner-{alert.tier}"
		role="alertdialog"
		tabindex="-1"
		aria-modal="true"
		data-blocks-escape="true"
		aria-labelledby="wx-banner-title"
		aria-describedby="wx-banner-desc"
		bind:this={el}
		onkeydown={trapTab}
		transition:fly={{ y: 24, duration: 250 }}
	>
		<div class="wx-banner-band" class:wx-band-hatched={alert.severity === 'Extreme' || alert.severity === 'Severe'} style="background: var({meta.colorVar})" aria-hidden="true"></div>
		<div class="wx-banner-head">
			<WxTierGlyph tier={alert.tier} size={22} />
			<h2 id="wx-banner-title" class="wx-banner-title">{alert.event}</h2>
			<span class="wx-banner-sub">In your watch area</span>
		</div>
		<p id="wx-banner-desc" class="wx-banner-body">{body}</p>
		<p class="wx-banner-affects">Affects {alert.affects.summary || 'the watch area'}</p>
		<p class="wx-banner-ends">Ends {clock(alert.endsAt)} · {cd.text}</p>
		{#if alert.instruction}<p class="wx-banner-instr">{alert.instruction}</p>{/if}
		<button class="wx-banner-ack" style="background: var({meta.colorVar})" bind:this={ackBtn} onclick={handleAck}>
			ACKNOWLEDGE
		</button>
		<div class="wx-banner-row">
			<button class="wx-banner-btn" onclick={() => alert && showAlertOnMap(alert.id)}>Show on map</button>
			<button class="wx-banner-btn" onclick={() => alert && openWxAlert(alert.id)}>Details</button>
			{#if $wxIsNcs}
				<button class="wx-banner-btn" onclick={handleAckForNet}>Ack for net</button>
			{/if}
		</div>
		<footer class="wx-banner-foot">
			<span>
				{alert.senderName} ·
				{$wxLinkStatus.fromCache ? `From cache · last fetched ${clock(alert.fetchedAt)}` : `fetched ${clockWithSeconds(alert.fetchedAt)}`}
			</span>
			{#if $wxInterruptMore > 0}
				<button class="wx-banner-more" onclick={rotateQueue}>+{$wxInterruptMore} more ▸</button>
			{/if}
		</footer>
	</div>
{/if}

<style>
	.wx-banner {
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
	}

	.wx-banner-warning { border: 1px solid var(--color-wx-warning); }
	.wx-banner-watch { border: 1px solid var(--color-wx-watch); }
	.wx-banner-advisory { border: 1px solid var(--color-wx-advisory); }
	.wx-banner-statement { border: 1px solid var(--color-wx-statement); }

	@media (min-width: 769px) {
		.wx-banner {
			right: var(--space-lg);
			bottom: calc(var(--space-lg) + 56px);
			max-width: 480px;
			width: calc(100vw - 2 * var(--space-lg));
		}
	}

	@media (max-width: 768px) {
		.wx-banner {
			left: var(--space-md);
			right: var(--space-md);
			bottom: calc(var(--sheet-peek) + var(--space-md) + env(safe-area-inset-bottom));
		}
	}

	.wx-banner-band {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		height: 6px;
	}

	.wx-band-hatched {
		background-image: repeating-linear-gradient(45deg, rgba(255, 255, 255, 0.35) 0, rgba(255, 255, 255, 0.35) 2px, transparent 2px, transparent 6px);
	}

	.wx-banner-head {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
	}

	.wx-banner-title {
		flex: 1;
		min-width: 0;
		font-size: 1.05rem;
		font-weight: 700;
	}

	.wx-banner-sub {
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-text-muted);
	}

	.wx-banner-body {
		font-size: 0.85rem;
		line-height: 1.4;
	}

	.wx-banner-affects,
	.wx-banner-ends {
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.wx-banner-instr {
		font-size: 0.8rem;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.wx-banner-ack {
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

	.wx-banner-watch .wx-banner-ack {
		color: #000;
	}

	.wx-banner-row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-xs);
	}

	.wx-banner-btn {
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

	.wx-banner-btn:hover,
	.wx-banner-btn:focus-visible {
		border-color: var(--color-accent);
	}

	.wx-banner-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-sm);
		margin-top: var(--space-xs);
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}

	.wx-banner-more {
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
