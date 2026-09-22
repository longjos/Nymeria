<script lang="ts">
	// Thin wrapper around InterruptBanner.svelte (spec §12: "Never build a
	// second full-width interrupt banner"). Builds an InterruptItem from the
	// wx stores; InterruptBanner owns the actual chrome, focus, chime and Tab
	// trap behaviour.
	import InterruptBanner from './InterruptBanner.svelte';
	import type { InterruptItem } from '$lib/stores/interrupts';
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

	let alert = $derived($wxActiveInterrupt);
	let meta = $derived(alert ? tierMeta[alert.tier] : null);
	let cd = $derived(alert ? countdown(alert.endsAt, $wxClock) : null);
	let body = $derived(alert ? announceSentence(alert, clockWithSeconds(alert.fetchedAt), $wxClock) : '');

	let item = $derived<InterruptItem | null>(
		alert && meta && cd
			? {
					id: alert.id,
					source: 'wx',
					title: alert.event,
					sub: 'In your watch area',
					body,
					where: `Affects ${alert.affects.summary || 'the watch area'}`,
					endsOrAt: `Ends ${clock(alert.endsAt)} · ${cd.text}`,
					instruction: alert.instruction || undefined,
					footer: `${alert.senderName} · ${
						$wxLinkStatus.fromCache ? `From cache · last fetched ${clock(alert.fetchedAt)}` : `fetched ${clockWithSeconds(alert.fetchedAt)}`
					}`,
					borderVar: meta.colorVar,
					hatched: alert.severity === 'Extreme' || alert.severity === 'Severe',
					glyph: { kind: 'wx', tier: alert.tier },
					ackTextDark: alert.tier === 'watch',
					showOnMap: () => showAlertOnMap(alert.id),
					details: () => openWxAlert(alert.id),
					ackForNet: $wxIsNcs ? () => ackAlertForNet(alert.id) : undefined
				}
			: null
	);

	async function handleAck(): Promise<void> {
		if (alert) await ackAlert(alert.id);
	}

	function rotateQueue(): void {
		wxInterruptQueue.update((q) => (q.length > 1 ? [...q.slice(1), q[0]] : q));
	}
</script>

<InterruptBanner {item} more={$wxInterruptMore} sounds={$wxLinkStatus.sounds} onAck={handleAck} onRotate={rotateQueue} />
