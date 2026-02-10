<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { closePanel } from '$lib/stores/ui';
	import { weatherConfig } from '$lib/stores/weather';
	import type {
		SettingsResponse, StationSettings, ServerSettings, BeaconSettings,
		SessionSettings, LoggingSettings, TransportSettings, TileCacheSettings,
		WeatherSettings
	} from '$lib/types';

	let loading = $state(true);
	let error = $state<string | null>(null);
	let settings = $state<SettingsResponse | null>(null);
	let restartBanner = $state(false);

	// Section open states
	let openSections = $state<Record<string, boolean>>({
		station: true,
		transports: false,
		beacon: false,
		session: false,
		logging: false,
		weather: false,
		tilecache: false,
		server: false,
		database: false
	});

	// Per-section saving/toast state
	let saving = $state<Record<string, boolean>>({});
	let toast = $state<{ section: string; message: string; type: 'success' | 'error' } | null>(null);

	function showToast(section: string, message: string, type: 'success' | 'error') {
		toast = { section, message, type };
		setTimeout(() => { if (toast?.section === section) toast = null; }, 3000);
	}

	async function loadSettings() {
		loading = true;
		error = null;
		try {
			settings = await api.getSettings();
		} catch (e: any) {
			error = e.message || 'Failed to load settings';
		} finally {
			loading = false;
		}
	}

	onMount(loadSettings);

	function toggle(section: string) {
		openSections[section] = !openSections[section];
	}

	// --- Save handlers ---

	async function saveStation() {
		if (!settings) return;
		saving['station'] = true;
		try {
			const resp = await api.updateStation(settings.station);
			if (resp.restartRequired) restartBanner = true;
			showToast('station', 'Station settings saved', 'success');
		} catch (e: any) {
			showToast('station', e.message || 'Save failed', 'error');
		} finally {
			saving['station'] = false;
		}
	}

	async function saveServer() {
		if (!settings) return;
		saving['server'] = true;
		try {
			const resp = await api.updateServer(settings.server);
			if (resp.restartRequired) restartBanner = true;
			showToast('server', 'Server settings saved', 'success');
		} catch (e: any) {
			showToast('server', e.message || 'Save failed', 'error');
		} finally {
			saving['server'] = false;
		}
	}

	async function saveTransports() {
		if (!settings) return;
		saving['transports'] = true;
		try {
			const resp = await api.updateTransports(settings.transports);
			if (resp.restartRequired) restartBanner = true;
			showToast('transports', 'Transport settings saved', 'success');
		} catch (e: any) {
			showToast('transports', e.message || 'Save failed', 'error');
		} finally {
			saving['transports'] = false;
		}
	}

	async function saveBeacon() {
		if (!settings) return;
		saving['beacon'] = true;
		try {
			const resp = await api.updateBeacon(settings.beacon);
			if (resp.restartRequired) restartBanner = true;
			showToast('beacon', 'Beacon settings saved', 'success');
		} catch (e: any) {
			showToast('beacon', e.message || 'Save failed', 'error');
		} finally {
			saving['beacon'] = false;
		}
	}

	async function saveSession() {
		if (!settings) return;
		saving['session'] = true;
		try {
			const resp = await api.updateSession(settings.session);
			if (resp.restartRequired) restartBanner = true;
			showToast('session', 'Session settings saved', 'success');
		} catch (e: any) {
			showToast('session', e.message || 'Save failed', 'error');
		} finally {
			saving['session'] = false;
		}
	}

	async function saveLogging() {
		if (!settings) return;
		saving['logging'] = true;
		try {
			const resp = await api.updateLogging(settings.logging);
			if (resp.restartRequired) restartBanner = true;
			showToast('logging', 'Logging settings saved', 'success');
		} catch (e: any) {
			showToast('logging', e.message || 'Save failed', 'error');
		} finally {
			saving['logging'] = false;
		}
	}

	async function saveWeather() {
		if (!settings) return;
		saving['weather'] = true;
		try {
			const resp = await api.updateWeather(settings.weather);
			if (resp.restartRequired) restartBanner = true;
			// Update the live weather config store so all components pick up the new units
			weatherConfig.set({
				retentionDays: settings.weather.retentionDays,
				alerts: settings.weather.alerts,
				units: (settings.weather.units === 'imperial' ? 'imperial' : 'metric')
			});
			showToast('weather', 'Weather settings saved', 'success');
		} catch (e: any) {
			showToast('weather', e.message || 'Save failed', 'error');
		} finally {
			saving['weather'] = false;
		}
	}

	async function saveTileCache() {
		if (!settings) return;
		saving['tilecache'] = true;
		try {
			const resp = await api.updateTileCache(settings.tileCache);
			if (resp.restartRequired) restartBanner = true;
			showToast('tilecache', 'Tile cache settings saved', 'success');
		} catch (e: any) {
			showToast('tilecache', e.message || 'Save failed', 'error');
		} finally {
			saving['tilecache'] = false;
		}
	}

	// --- Transport helpers ---

	function addTransport(type: string) {
		if (!settings) return;
		const t: TransportSettings = { type };
		if (type === 'aprsis') {
			t.host = 'rotate.aprs2.net';
			t.port = 14580;
		} else if (type === 'kisstcp') {
			t.host = 'localhost';
			t.port = 8001;
		} else if (type === 'serial') {
			t.device = '/dev/ttyUSB0';
			t.baud = 9600;
		}
		settings.transports = [...settings.transports, t];
	}

	function removeTransport(index: number) {
		if (!settings) return;
		settings.transports = settings.transports.filter((_, i) => i !== index);
	}

	let showPIN = $state(false);
	let addTransportType = $state<string | null>(null);
</script>

<div class="settings-panel">
	<!-- Header -->
	<div class="panel-header">
		<h2>Settings</h2>
		<button class="close-btn" onclick={closePanel} aria-label="Close">
			<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
				<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
			</svg>
		</button>
	</div>

	<!-- Restart banner -->
	{#if restartBanner}
		<div class="restart-banner">
			<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
				<path d="M8 1l7 14H1L8 1z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>
				<path d="M8 6v3M8 11.5h.01" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
			</svg>
			<span>Some changes require a restart to take effect.</span>
			<button class="banner-dismiss" onclick={() => { restartBanner = false; }}>Dismiss</button>
		</div>
	{/if}

	<!-- Toast notification -->
	{#if toast}
		<div class="toast" class:toast-error={toast.type === 'error'} class:toast-success={toast.type === 'success'}>
			{toast.message}
		</div>
	{/if}

	{#if loading}
		<div class="loading">
			<div class="skeleton"></div>
			<div class="skeleton"></div>
			<div class="skeleton"></div>
		</div>
	{:else if error}
		<div class="error-state">
			<p>{error}</p>
			<button onclick={loadSettings}>Retry</button>
		</div>
	{:else if settings}
		<div class="sections">
			<!-- Station -->
			<div class="section" class:open={openSections.station}>
				<button class="section-header" onclick={() => toggle('station')}>
					<span class="section-title">Station</span>
					<span class="section-badge live">Live</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.station}
					<div class="section-body">
						<div class="field-row">
							<label for="st-callsign">Callsign</label>
							<input id="st-callsign" type="text" bind:value={settings.station.callsign} placeholder="W1AW" />
						</div>
						<div class="field-row">
							<label for="st-ssid">SSID</label>
							<input id="st-ssid" type="number" min="0" max="15" bind:value={settings.station.ssid} />
						</div>
						<div class="field-group">
							<div class="field-row half">
								<label for="st-lat">Latitude</label>
								<input id="st-lat" type="number" step="0.0001" bind:value={settings.station.lat} />
							</div>
							<div class="field-row half">
								<label for="st-lon">Longitude</label>
								<input id="st-lon" type="number" step="0.0001" bind:value={settings.station.lon} />
							</div>
						</div>
						<div class="field-group">
							<div class="field-row half">
								<label for="st-symtable">Symbol Table</label>
								<input id="st-symtable" type="text" maxlength="1" bind:value={settings.station.symbolTable} placeholder="/" />
							</div>
							<div class="field-row half">
								<label for="st-symcode">Symbol Code</label>
								<input id="st-symcode" type="text" maxlength="1" bind:value={settings.station.symbolCode} placeholder="-" />
							</div>
						</div>
						<div class="field-row">
							<label for="st-comment">Comment</label>
							<input id="st-comment" type="text" bind:value={settings.station.comment} />
						</div>
						<div class="field-row">
							<label for="st-maxpts">Track Max Points</label>
							<input id="st-maxpts" type="number" min="0" bind:value={settings.station.trackMaxPoints} />
						</div>
						<div class="field-group">
							<div class="field-row half">
								<label for="st-stale">Stale Timeout</label>
								<input id="st-stale" type="text" bind:value={settings.station.staleTimeout} placeholder="1h20m0s" />
							</div>
							<div class="field-row half">
								<label for="st-dedup">Dedup Window</label>
								<input id="st-dedup" type="text" bind:value={settings.station.dedupWindow} placeholder="30s" />
							</div>
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveStation} disabled={saving['station']}>
								{saving['station'] ? 'Saving...' : 'Save Station'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Transports -->
			<div class="section" class:open={openSections.transports}>
				<button class="section-header" onclick={() => toggle('transports')}>
					<span class="section-title">Transports</span>
					<span class="section-badge live">Live</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.transports}
					<div class="section-body">
						{#each settings.transports as t, i}
							<div class="transport-card">
								<div class="transport-header">
									<span class="transport-type">{t.type.toUpperCase()}</span>
									<button class="remove-btn" onclick={() => removeTransport(i)} title="Remove transport">
										<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
											<path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
										</svg>
									</button>
								</div>
								{#if t.type === 'aprsis'}
									<div class="field-group">
										<div class="field-row half">
											<label>Host</label>
											<input type="text" bind:value={t.host} />
										</div>
										<div class="field-row half">
											<label>Port</label>
											<input type="number" bind:value={t.port} />
										</div>
									</div>
									<div class="field-row">
										<label>Filter</label>
										<input type="text" bind:value={t.filter} placeholder="r/42/-71/100" />
									</div>
									<div class="field-group">
										<div class="field-row half">
											<label>Callsign</label>
											<input type="text" bind:value={t.callsign} placeholder="Override" />
										</div>
										<div class="field-row half">
											<label>Passcode</label>
											<input type="password" bind:value={t.passcode} placeholder="***" />
										</div>
									</div>
								{:else if t.type === 'kisstcp'}
									<div class="field-group">
										<div class="field-row half">
											<label>Host</label>
											<input type="text" bind:value={t.host} />
										</div>
										<div class="field-row half">
											<label>Port</label>
											<input type="number" bind:value={t.port} />
										</div>
									</div>
								{:else if t.type === 'serial'}
									<div class="field-group">
										<div class="field-row half">
											<label>Device</label>
											<input type="text" bind:value={t.device} placeholder="/dev/ttyUSB0" />
										</div>
										<div class="field-row half">
											<label>Baud</label>
											<input type="number" bind:value={t.baud} />
										</div>
									</div>
								{/if}
							</div>
						{/each}

						<div class="add-transport">
							{#if addTransportType}
								<div class="type-picker">
									<button class="type-option" onclick={() => { addTransport('aprsis'); addTransportType = null; }}>APRS-IS</button>
									<button class="type-option" onclick={() => { addTransport('kisstcp'); addTransportType = null; }}>KISS TCP</button>
									<button class="type-option" onclick={() => { addTransport('serial'); addTransportType = null; }}>Serial</button>
									<button class="type-cancel" onclick={() => { addTransportType = null; }}>Cancel</button>
								</div>
							{:else}
								<button class="add-btn" onclick={() => { addTransportType = 'pick'; }}>+ Add Transport</button>
							{/if}
						</div>

						<div class="section-actions">
							<button class="save-btn" onclick={saveTransports} disabled={saving['transports']}>
								{saving['transports'] ? 'Saving...' : 'Save Transports'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Beacon -->
			<div class="section" class:open={openSections.beacon}>
				<button class="section-header" onclick={() => toggle('beacon')}>
					<span class="section-title">Beacon</span>
					<span class="section-badge live">Live</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.beacon}
					<div class="section-body">
						<div class="field-row toggle-row">
							<label for="bcn-enabled">Enabled</label>
							<input id="bcn-enabled" type="checkbox" bind:checked={settings.beacon.enabled} />
						</div>
						<div class="field-row">
							<label for="bcn-interval">Interval</label>
							<input id="bcn-interval" type="text" bind:value={settings.beacon.interval} placeholder="10m0s" />
						</div>
						<div class="field-row">
							<label for="bcn-comment">Comment</label>
							<input id="bcn-comment" type="text" bind:value={settings.beacon.comment} />
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveBeacon} disabled={saving['beacon']}>
								{saving['beacon'] ? 'Saving...' : 'Save Beacon'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Session -->
			<div class="section" class:open={openSections.session}>
				<button class="section-header" onclick={() => toggle('session')}>
					<span class="section-title">Session</span>
					<span class="section-badge live">Live</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.session}
					<div class="section-body">
						<div class="field-row">
							<label for="sess-pin">PIN</label>
							<div class="pin-field">
								{#if showPIN}
									<input id="sess-pin" type="text" bind:value={settings.session.pin} placeholder="No PIN set" />
								{:else}
									<input id="sess-pin" type="password" bind:value={settings.session.pin} placeholder="No PIN set" />
								{/if}
								<button class="pin-toggle" onclick={() => { showPIN = !showPIN; }}>
									{showPIN ? 'Hide' : 'Show'}
								</button>
							</div>
						</div>
						<div class="field-row">
							<label for="sess-timeout">Inactivity Timeout</label>
							<input id="sess-timeout" type="text" bind:value={settings.session.inactivityTimeout} placeholder="30m0s" />
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveSession} disabled={saving['session']}>
								{saving['session'] ? 'Saving...' : 'Save Session'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Logging -->
			<div class="section" class:open={openSections.logging}>
				<button class="section-header" onclick={() => toggle('logging')}>
					<span class="section-title">Logging</span>
					<span class="section-badge live">Live</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.logging}
					<div class="section-body">
						<div class="field-row">
							<label for="log-level">Level</label>
							<select id="log-level" bind:value={settings.logging.level}>
								<option value="debug">Debug</option>
								<option value="info">Info</option>
								<option value="warn">Warn</option>
								<option value="error">Error</option>
							</select>
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveLogging} disabled={saving['logging']}>
								{saving['logging'] ? 'Saving...' : 'Save Logging'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Weather -->
			<div class="section" class:open={openSections.weather}>
				<button class="section-header" onclick={() => toggle('weather')}>
					<span class="section-title">Weather</span>
					<span class="section-badge live">Live</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.weather}
					<div class="section-body">
						<div class="field-row">
							<label>Units</label>
							<div class="unit-toggle">
								<button
									class="unit-option"
									class:active={settings.weather.units !== 'imperial'}
									onclick={() => { if (settings) settings.weather.units = 'metric'; }}
								>Metric</button>
								<button
									class="unit-option"
									class:active={settings.weather.units === 'imperial'}
									onclick={() => { if (settings) settings.weather.units = 'imperial'; }}
								>Imperial</button>
							</div>
						</div>
						<div class="field-row">
							<label for="wx-retention">Retention Days</label>
							<input id="wx-retention" type="number" min="1" bind:value={settings.weather.retentionDays} />
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveWeather} disabled={saving['weather']}>
								{saving['weather'] ? 'Saving...' : 'Save Weather'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Tile Cache -->
			<div class="section" class:open={openSections.tilecache}>
				<button class="section-header" onclick={() => toggle('tilecache')}>
					<span class="section-title">Tile Cache</span>
					<span class="section-badge restart">Restart</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.tilecache}
					<div class="section-body">
						<div class="field-row toggle-row">
							<label for="tc-enabled">Enabled</label>
							<input id="tc-enabled" type="checkbox" bind:checked={settings.tileCache.enabled} />
						</div>
						<div class="field-row">
							<label for="tc-dir">Data Directory</label>
							<input id="tc-dir" type="text" bind:value={settings.tileCache.dataDir} placeholder="./tiles" />
						</div>
						<div class="field-row">
							<label for="tc-url">Tile URL</label>
							<input id="tc-url" type="text" bind:value={settings.tileCache.tileUrl} />
						</div>
						<div class="field-row">
							<label for="tc-zoom">Max Zoom</label>
							<input id="tc-zoom" type="number" min="1" max="20" bind:value={settings.tileCache.maxZoom} />
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveTileCache} disabled={saving['tilecache']}>
								{saving['tilecache'] ? 'Saving...' : 'Save Tile Cache'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Server -->
			<div class="section" class:open={openSections.server}>
				<button class="section-header" onclick={() => toggle('server')}>
					<span class="section-title">Server</span>
					<span class="section-badge restart">Restart</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.server}
					<div class="section-body">
						<div class="field-row">
							<label for="srv-listen">Listen Address</label>
							<input id="srv-listen" type="text" bind:value={settings.server.listen} placeholder=":8080" />
						</div>
						<div class="section-actions">
							<button class="save-btn" onclick={saveServer} disabled={saving['server']}>
								{saving['server'] ? 'Saving...' : 'Save Server'}
							</button>
						</div>
					</div>
				{/if}
			</div>

			<!-- Database (read-only) -->
			<div class="section" class:open={openSections.database}>
				<button class="section-header" onclick={() => toggle('database')}>
					<span class="section-title">Database</span>
					<span class="section-badge readonly">Read-only</span>
					<svg class="chevron" width="14" height="14" viewBox="0 0 16 16" fill="none">
						<path d="M4 6l4 4 4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
				{#if openSections.database}
					<div class="section-body">
						<div class="field-row">
							<label>Path</label>
							<div class="readonly-value">{settings.store.path}</div>
						</div>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	.settings-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
	}

	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.panel-header h2 {
		font-size: 1rem;
		font-weight: 600;
		margin: 0;
		color: var(--color-text);
	}

	.close-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.close-btn:hover {
		color: var(--color-text);
		background: var(--color-primary);
	}

	/* Restart banner */
	.restart-banner {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		padding: var(--space-sm) var(--space-md);
		background: rgba(234, 179, 8, 0.12);
		border-bottom: 1px solid rgba(234, 179, 8, 0.3);
		color: #fbbf24;
		font-size: 0.78rem;
		flex-shrink: 0;
	}

	.restart-banner span {
		flex: 1;
	}

	.banner-dismiss {
		background: none;
		border: 1px solid rgba(234, 179, 8, 0.3);
		color: #fbbf24;
		border-radius: var(--radius-sm);
		padding: 2px 8px;
		font-size: 0.7rem;
		cursor: pointer;
	}

	.banner-dismiss:hover {
		background: rgba(234, 179, 8, 0.15);
	}

	/* Toast */
	.toast {
		position: absolute;
		top: 60px;
		left: 50%;
		transform: translateX(-50%);
		padding: 6px 16px;
		border-radius: var(--radius-md);
		font-size: 0.78rem;
		font-weight: 500;
		z-index: 10;
		animation: toastIn 0.2s ease-out;
	}

	.toast-success {
		background: rgba(34, 197, 94, 0.15);
		border: 1px solid rgba(34, 197, 94, 0.3);
		color: #4ade80;
	}

	.toast-error {
		background: rgba(239, 68, 68, 0.15);
		border: 1px solid rgba(239, 68, 68, 0.3);
		color: #f87171;
	}

	@keyframes toastIn {
		from { opacity: 0; transform: translateX(-50%) translateY(-8px); }
		to { opacity: 1; transform: translateX(-50%) translateY(0); }
	}

	/* Loading */
	.loading {
		padding: var(--space-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-md);
	}

	.skeleton {
		height: 48px;
		border-radius: var(--radius-md);
		background: var(--color-primary);
		animation: shimmer 1.5s infinite;
	}

	@keyframes shimmer {
		0% { opacity: 0.5; }
		50% { opacity: 0.8; }
		100% { opacity: 0.5; }
	}

	.error-state {
		padding: var(--space-lg);
		text-align: center;
		color: var(--color-text-muted);
	}

	.error-state button {
		margin-top: var(--space-md);
		padding: 6px 16px;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	/* Sections */
	.sections {
		flex: 1;
		overflow-y: auto;
		padding-bottom: var(--space-lg);
	}

	.section {
		border-bottom: 1px solid var(--color-primary);
	}

	.section-header {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
		width: 100%;
		padding: var(--space-sm) var(--space-md);
		background: none;
		border: none;
		color: var(--color-text);
		cursor: pointer;
		font-size: 0.85rem;
		font-weight: 600;
		text-align: left;
		transition: background var(--duration-fast);
	}

	.section-header:hover {
		background: rgba(255, 255, 255, 0.03);
	}

	.section-title {
		flex: 1;
	}

	.section-badge {
		font-size: 0.6rem;
		font-weight: 600;
		padding: 1px 6px;
		border-radius: 10px;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.section-badge.live {
		background: rgba(34, 197, 94, 0.15);
		color: #4ade80;
		border: 1px solid rgba(34, 197, 94, 0.25);
	}

	.section-badge.restart {
		background: rgba(234, 179, 8, 0.12);
		color: #fbbf24;
		border: 1px solid rgba(234, 179, 8, 0.25);
	}

	.section-badge.readonly {
		background: rgba(148, 163, 184, 0.1);
		color: var(--color-text-muted);
		border: 1px solid rgba(148, 163, 184, 0.2);
	}

	.chevron {
		transition: transform var(--duration-fast);
		color: var(--color-text-muted);
		flex-shrink: 0;
	}

	.section.open .chevron {
		transform: rotate(180deg);
	}

	.section-body {
		padding: 0 var(--space-md) var(--space-md);
	}

	/* Fields */
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

	.field-row input[type="text"],
	.field-row input[type="number"],
	.field-row input[type="password"],
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
		transition: border-color var(--duration-fast);
	}

	.field-row input:focus,
	.field-row select:focus {
		border-color: var(--color-accent);
	}

	.field-row select {
		appearance: none;
		cursor: pointer;
	}

	.field-group {
		display: flex;
		gap: var(--space-sm);
	}

	.field-row.half {
		flex: 1;
	}

	.toggle-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-direction: row;
	}

	.toggle-row label {
		margin-bottom: 0;
	}

	.toggle-row input[type="checkbox"] {
		width: 36px;
		height: 20px;
		appearance: none;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.15);
		border-radius: 10px;
		cursor: pointer;
		position: relative;
		transition: background var(--duration-fast);
	}

	.toggle-row input[type="checkbox"]::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 14px;
		height: 14px;
		background: var(--color-text-muted);
		border-radius: 50%;
		transition: transform var(--duration-fast), background var(--duration-fast);
	}

	.toggle-row input[type="checkbox"]:checked {
		background: rgba(34, 197, 94, 0.2);
		border-color: rgba(34, 197, 94, 0.4);
	}

	.toggle-row input[type="checkbox"]:checked::after {
		transform: translateX(16px);
		background: #4ade80;
	}

	.readonly-value {
		padding: 6px 10px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.05);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		font-size: 0.82rem;
		font-family: monospace;
	}

	/* PIN field */
	.pin-field {
		display: flex;
		gap: var(--space-xs);
	}

	.pin-field input {
		flex: 1;
		padding: 6px 10px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		font-size: 0.82rem;
		font-family: inherit;
		outline: none;
	}

	.pin-field input:focus {
		border-color: var(--color-accent);
	}

	.pin-toggle {
		padding: 4px 8px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.08);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.7rem;
		white-space: nowrap;
	}

	.pin-toggle:hover {
		color: var(--color-text);
	}

	/* Section actions */
	.section-actions {
		margin-top: var(--space-sm);
		display: flex;
		justify-content: flex-end;
	}

	.save-btn {
		padding: 6px 16px;
		background: var(--color-accent);
		color: white;
		border: none;
		border-radius: var(--radius-sm);
		font-size: 0.78rem;
		font-weight: 600;
		cursor: pointer;
		transition: opacity var(--duration-fast);
	}

	.save-btn:hover:not(:disabled) {
		opacity: 0.9;
	}

	.save-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Transport cards */
	.transport-card {
		background: rgba(255, 255, 255, 0.02);
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: var(--radius-md);
		padding: var(--space-sm);
		margin-bottom: var(--space-sm);
	}

	.transport-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: var(--space-sm);
	}

	.transport-type {
		font-size: 0.72rem;
		font-weight: 700;
		color: var(--color-accent);
		letter-spacing: 0.05em;
	}

	.remove-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		background: none;
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
	}

	.remove-btn:hover {
		color: #f87171;
		background: rgba(239, 68, 68, 0.1);
	}

	/* Add transport */
	.add-transport {
		margin-bottom: var(--space-sm);
	}

	.add-btn {
		width: 100%;
		padding: 8px;
		background: none;
		border: 1px dashed rgba(255, 255, 255, 0.15);
		border-radius: var(--radius-md);
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.78rem;
		transition: border-color var(--duration-fast), color var(--duration-fast);
	}

	.add-btn:hover {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.type-picker {
		display: flex;
		gap: var(--space-xs);
		flex-wrap: wrap;
	}

	.type-option {
		flex: 1;
		min-width: 70px;
		padding: 6px;
		background: var(--color-primary);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		cursor: pointer;
		font-size: 0.75rem;
		font-weight: 600;
		text-align: center;
	}

	.type-option:hover {
		border-color: var(--color-accent);
		color: var(--color-accent);
	}

	.type-cancel {
		padding: 6px;
		background: none;
		border: 1px solid rgba(255, 255, 255, 0.06);
		border-radius: var(--radius-sm);
		color: var(--color-text-muted);
		cursor: pointer;
		font-size: 0.75rem;
	}

	/* Unit toggle */
	.unit-toggle {
		display: flex;
		gap: 2px;
		background: var(--color-primary);
		border-radius: var(--radius-sm);
		padding: 2px;
	}

	.unit-option {
		flex: 1;
		padding: 5px 12px;
		font-size: 0.78rem;
		font-weight: 600;
		background: none;
		border: none;
		border-radius: 4px;
		color: var(--color-text-muted);
		cursor: pointer;
		transition: background var(--duration-fast), color var(--duration-fast);
	}

	.unit-option.active {
		background: var(--color-accent);
		color: white;
	}

	.unit-option:hover:not(.active) {
		color: var(--color-text);
	}
</style>
