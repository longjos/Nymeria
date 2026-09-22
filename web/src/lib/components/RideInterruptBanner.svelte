<script lang="ts">
	// Thin wrapper around InterruptBanner.svelte for ride EMERGENCY (spec §0/§4).
	// Builds an InterruptItem from the ride stores — never PatientName or Bib,
	// even for an operator (medical data may already be redacted server-side;
	// this component never assumes otherwise).
	import InterruptBanner from './InterruptBanner.svelte';
	import type { InterruptItem } from '$lib/stores/interrupts';
	import { rideActiveInterrupt, ackEmergency } from '$lib/stores/ride';
	import { rideInterruptQueue } from '$lib/stores/interrupts';
	import { tierStyle } from '$lib/rideMeta';
	import { openNetControl, netControlRequestedTab } from '$lib/stores/ui';

	let item = $derived<InterruptItem | null>(
		$rideActiveInterrupt
			? {
					id: $rideActiveInterrupt.id,
					source: 'ride',
					title: `${$rideActiveInterrupt.kind.toUpperCase()} · ${$rideActiveInterrupt.tier.label.toUpperCase()}`,
					sub: 'Ride net',
					body: $rideActiveInterrupt.summary,
					where: $rideActiveInterrupt.where,
					footer: `Reported ${new Date($rideActiveInterrupt.createdAt).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}`,
					borderVar: tierStyle($rideActiveInterrupt.tier).colorVar,
					hatched: true,
					glyph: { kind: 'ride', tier: $rideActiveInterrupt.tier },
					repeatEveryMs: 60_000,
					details: () => {
						netControlRequestedTab.set('situation');
						openNetControl();
					}
				}
			: null
	);

	let more = $derived(Math.max(0, $rideInterruptQueue.length - 1));

	async function handleAck(): Promise<void> {
		if ($rideActiveInterrupt) await ackEmergency($rideActiveInterrupt.id);
	}

	function rotateQueue(): void {
		rideInterruptQueue.update((q) => (q.length > 1 ? [...q.slice(1), q[0]] : q));
	}
</script>

<InterruptBanner {item} {more} sounds={true} onAck={handleAck} onRotate={rotateQueue} />
