<script lang="ts">
	import type { SerialPortInfo, SerialProfile, TransportStatus } from '$lib/types';
	import {
		persistDevice,
		normalizeDevice,
		mergePortOptions,
		inferProfile,
		emptyStateKind,
		isLocalHost,
		highlightedPorts,
		MANUAL_PORT_VALUE,
		kissLinkState,
		kissLinkLabel,
		kissLinkHint
	} from '$lib/serialPorts';

	let {
		idPrefix,
		device = $bindable(''),
		baud = $bindable(9600),
		ports = [],
		hostOS = '',
		profiles = [],
		baudRates = [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200],
		portsLoading = false,
		portsError = null,
		onrefresh,
		liveStatus = null
	}: {
		idPrefix: string;
		device: string | undefined;
		baud: number | undefined;
		ports: SerialPortInfo[];
		hostOS: string;
		profiles: SerialProfile[];
		baudRates: number[];
		portsLoading: boolean;
		portsError: string | null;
		onrefresh: () => void;
		liveStatus?: TransportStatus | null;
	} = $props();

	const VCP_URL = 'https://www.kenwood.com/i/products/info/amateur/thd74_vcp_e.html';

	let forceCustomBaud = $state(false);
	let profileOverride = $state<string | null>(null);
	let manualMode = $state(false);

	let merged = $derived(mergePortOptions(ports, device ?? ''));
	let selectedPath = $derived(normalizeDevice(device ?? ''));

	let selectedOption = $derived.by(() => {
		if (manualMode) return MANUAL_PORT_VALUE;
		if (!selectedPath) return '';
		const hit = merged.find(
			(p) => normalizeDevice(p.name) === selectedPath || normalizeDevice(p.stablePath ?? '') === selectedPath
		);
		return hit ? persistDevice(hit) : selectedPath ? MANUAL_PORT_VALUE : '';
	});

	let baudIsStandard = $derived(baudRates.includes(baud));
	let baudSelect = $derived(forceCustomBaud || !baudIsStandard ? 'custom' : String(baud));

	let profileId = $derived.by(() => {
		const port = merged.find(
			(p) => persistDevice(p) === persistDevice({ name: selectedPath, stablePath: undefined }) ||
				normalizeDevice(p.name) === selectedPath ||
				normalizeDevice(p.stablePath ?? '') === selectedPath
		);
		return inferProfile(port, baud || 0, profiles, profileOverride ?? undefined);
	});

	let profile = $derived(profiles.find((p) => p.id === profileId));

	let oneHighlight = $derived.by(() => {
		const hits = highlightedPorts(merged);
		return hits.length === 1 ? hits[0] : null;
	});

	let showUseThis = $derived(
		!!oneHighlight && (!selectedPath || merged.some((p) => !p.present && normalizeDevice(p.name) === selectedPath))
	);

	let rematch = $derived.by(() => {
		if (!selectedPath || !oneHighlight) return null;
		const missing = merged.some((p) => !p.present && normalizeDevice(p.name) === selectedPath);
		return missing ? oneHighlight : null;
	});

	let emptyKind = $derived(emptyStateKind(hostOS));
	let remoteHost = $derived(typeof location !== 'undefined' && !isLocalHost(location.hostname));

	function onPortChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (value === MANUAL_PORT_VALUE) {
			manualMode = true;
			return;
		}
		manualMode = false;
		device = normalizeDevice(value);
		profileOverride = null;
	}

	function onManualInput(event: Event) {
		device = normalizeDevice((event.currentTarget as HTMLInputElement).value);
	}

	function onBaudChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (value === 'custom') {
			forceCustomBaud = true;
			return;
		}
		forceCustomBaud = false;
		baud = Number(value);
	}

	function onCustomBaud(event: Event) {
		const n = Number((event.currentTarget as HTMLInputElement).value);
		if (!Number.isNaN(n)) baud = n;
	}

	function onProfileChange(event: Event) {
		const id = (event.currentTarget as HTMLSelectElement).value;
		profileOverride = id;
		const p = profiles.find((x) => x.id === id);
		if (p && !forceCustomBaud) {
			baud = p.baud;
		}
	}

	function useRadio(port: SerialPortInfo) {
		manualMode = false;
		device = persistDevice(port);
		profileOverride = null;
		const p = profiles.find((x) => x.id === port.suggestedProfile);
		if (p && !forceCustomBaud) baud = p.baud;
	}

	function optionLabel(p: SerialPortInfo): string {
		if (p.highlight && p.present !== false) return `${p.label} — suggested`;
		return p.label;
	}
</script>

<div class="serial-fields">
	<div class="field-row">
		<label for="{idPrefix}-profile">Device profile</label>
		<select id="{idPrefix}-profile" value={profileId} onchange={onProfileChange}>
			{#each profiles as p}
				<option value={p.id}>{p.label}</option>
			{/each}
		</select>
	</div>
	{#if profile?.help}
		<p class="field-help">{profile.help}</p>
	{/if}

	<div class="field-row port-row">
		<label for="{idPrefix}-dev">Serial port</label>
		<button type="button" class="refresh-btn" onclick={onrefresh} aria-label="Refresh serial ports" disabled={portsLoading}>
			{portsLoading ? '…' : 'Refresh'}
		</button>
		<select id="{idPrefix}-dev" value={selectedOption} onchange={onPortChange}>
			<option value="">Select a port…</option>
			{#each merged as p}
				<option value={persistDevice(p)}>{optionLabel(p)}</option>
			{/each}
			<option value={MANUAL_PORT_VALUE}>Enter path manually…</option>
		</select>
	</div>

	{#if selectedOption === MANUAL_PORT_VALUE || manualMode}
		<div class="field-row">
			<label for="{idPrefix}-dev-manual">Device path</label>
			<input
				id="{idPrefix}-dev-manual"
				type="text"
				value={device}
				oninput={onManualInput}
				placeholder={emptyKind === 'windows' ? 'COM5' : emptyKind === 'darwin' ? '/dev/cu.usbmodem*' : '/dev/ttyACM0'}
				spellcheck="false"
				autocomplete="off"
				autocapitalize="off"
			/>
		</div>
	{/if}

	{#if rematch}
		<div class="banner rematch">
			<span>{selectedPath} is not present. Use {rematch.label}?</span>
			<button type="button" class="banner-btn" onclick={() => useRadio(rematch)}>Use this port</button>
		</div>
	{:else if showUseThis && oneHighlight}
		<div class="banner suggest">
			<span>
				{oneHighlight.suggestedProfile === 'kenwood-thd7x-usb' ? 'Kenwood radio' : 'TNC'} detected as {oneHighlight.label}.
			</span>
			<button type="button" class="banner-btn" onclick={() => useRadio(oneHighlight)}>Use this radio</button>
		</div>
	{/if}

	{#if selectedPath && merged.some((p) => !p.present && normalizeDevice(p.name) === selectedPath) && !rematch}
		<p class="field-help warn">This port is not currently available. Plug it in and Refresh, or pick another.</p>
	{/if}

	{#if ports.length === 0 && !portsLoading}
		<div class="empty-ports">
			{#if remoteHost}
				<p class="field-help"><strong>Ports are listed on the Nymeria server, not this browser.</strong></p>
			{:else}
				<p class="field-help">Ports are listed on the Nymeria server, not this browser.</p>
			{/if}
			{#if emptyKind === 'windows'}
				<p class="field-help">
					Install the Kenwood USB CDC virtual COM driver <strong>before</strong> plugging in (not Silicon Labs CP210x),
					power the radio ON, then connect USB. Device Manager → Ports should show <code>TH-Dxx (COMxx)</code>.
					Menu 980 Mass Storage removes the COM port. Then Refresh.
				</p>
				<p class="field-help">
					<a href={VCP_URL} target="_blank" rel="noopener noreferrer">Kenwood VCP driver</a>
				</p>
			{:else if emptyKind === 'linux'}
				<p class="field-help">
					Is the radio powered and plugged in? The Nymeria user usually needs the <code>dialout</code> group
					(<code>sudo usermod -aG dialout $USER</code> then log out). CDC radios are <code>/dev/ttyACM*</code>.
				</p>
			{:else if emptyKind === 'darwin'}
				<p class="field-help">
					Plug in the radio (macOS does not use the Kenwood Windows VCP). After Refresh you should see
					<code>/dev/cu.usbmodem*</code>. Prefer <code>cu.*</code> over <code>tty.*</code>.
				</p>
			{:else}
				<p class="field-help">Plug in the TNC or radio, then Refresh.</p>
			{/if}
		</div>
	{/if}

	{#if portsError}
		<p class="field-help warn">{portsError}</p>
	{/if}

	<div class="field-row">
		<label for="{idPrefix}-baud">Baud</label>
		<select id="{idPrefix}-baud" value={baudSelect} onchange={onBaudChange}>
			{#each baudRates as rate}
				<option value={String(rate)}>{rate}</option>
			{/each}
			<option value="custom">Custom…</option>
		</select>
	</div>
	{#if baudSelect === 'custom'}
		<div class="field-row">
			<label for="{idPrefix}-baud-custom">Custom baud</label>
			<input id="{idPrefix}-baud-custom" type="number" value={baud} oninput={onCustomBaud} />
		</div>
	{/if}
	<p class="field-help">
		USB virtual COM often ignores baud; 9600 is fine. On a TH-D74/D75 the on-air rate is Menu 505 — not this setting.
	</p>

	{#if liveStatus}
		{@const link = kissLinkState(liveStatus)}
		<div class="link-status">
			<span class="chip port" class:on={liveStatus.connected} title="OS opened the serial port. Not a TNC handshake.">
				{liveStatus.connected ? 'Port open' : 'Port closed'}
			</span>
			<span class="chip {link}" title={kissLinkHint(link)}>{kissLinkLabel(link)}</span>
			<span class="counts" title="Decoded inbound KISS / writes we handed to the port">
				RX {liveStatus.packetsRx ?? 0} · TX {liveStatus.packetsTx ?? 0}
			</span>
		</div>
		{#if liveStatus.error && !liveStatus.connected}
			<p class="field-help warn">{liveStatus.error}</p>
		{:else if link === 'quiet'}
			<p class="field-help">{kissLinkHint(link)}</p>
		{/if}
	{/if}
</div>

<style>
	.port-row {
		position: relative;
	}

	.refresh-btn {
		position: absolute;
		top: 0;
		right: 0;
		background: none;
		border: none;
		color: var(--color-accent);
		font-size: 0.72rem;
		font-weight: 600;
		cursor: pointer;
		padding: 0;
	}

	.refresh-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.field-row {
		margin-bottom: var(--space-sm);
	}

	.field-row label {
		display: block;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		margin-bottom: 2px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.field-row input[type='text'],
	.field-row input[type='number'],
	.field-row select {
		width: 100%;
		padding: 6px 10px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.82rem;
		font-family: inherit;
		outline: none;
	}

	.field-row select {
		appearance: none;
		cursor: pointer;
	}

	.field-help {
		margin: 0 0 var(--space-sm);
		font-size: 0.72rem;
		color: var(--color-text-muted);
		line-height: 1.4;
	}

	.field-help.warn {
		color: #fbbf24;
	}

	.field-help code {
		font-size: 0.7rem;
	}

	.field-help a {
		color: var(--color-accent);
	}

	.banner {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-xs);
		margin-bottom: var(--space-sm);
		padding: 6px 8px;
		border-radius: var(--radius-sm);
		background: rgba(56, 189, 248, 0.08);
		border: 1px solid rgba(56, 189, 248, 0.2);
		font-size: 0.75rem;
		color: var(--color-text);
	}

	.banner-btn {
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		padding: 3px 8px;
		font-size: 0.72rem;
		font-weight: 600;
		cursor: pointer;
	}

	.empty-ports {
		margin-bottom: var(--space-sm);
	}

	.link-status {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
		margin: 0 0 var(--space-xs);
	}

	.chip {
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		padding: 2px 7px;
		border-radius: var(--radius-full);
		background: rgba(255, 255, 255, 0.06);
		color: var(--color-text-muted);
	}

	.chip.port.on {
		background: color-mix(in srgb, var(--color-connected) 18%, transparent);
		color: var(--color-connected);
	}

	.chip.kiss {
		background: color-mix(in srgb, var(--color-connected) 18%, transparent);
		color: var(--color-connected);
	}

	.chip.quiet {
		background: color-mix(in srgb, var(--color-warning) 18%, transparent);
		color: var(--color-warning);
	}

	.chip.error {
		background: color-mix(in srgb, var(--color-error) 18%, transparent);
		color: var(--color-error);
	}

	.counts {
		font-size: 0.72rem;
		font-variant-numeric: tabular-nums;
		color: var(--color-text-muted);
	}
</style>
