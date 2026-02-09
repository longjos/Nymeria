<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import type { ICS309Report, ICS309Row } from '$lib/types';
	import { ics309NetId, closePanel } from '$lib/stores/ui';

	let report = $state<ICS309Report | null>(null);
	let loading = $state(false);
	let error = $state('');

	// Editable header fields
	let incidentName = $state('');
	let dateFrom = $state('');
	let dateTo = $state('');
	let operatorName = $state('');
	let stationId = $state('');

	onMount(() => {
		loadReport();
	});

	async function loadReport() {
		loading = true;
		error = '';
		try {
			const params: Record<string, string> = {};
			if ($ics309NetId) params.netId = $ics309NetId;
			if (incidentName) params.incidentName = incidentName;
			if (dateFrom) params.from = dateFrom;
			if (dateTo) params.to = dateTo;
			if (operatorName) params.operatorName = operatorName;
			if (stationId) params.stationId = stationId;

			report = await api.ics309(params);

			// Populate editable fields from response
			if (report.header) {
				if (!incidentName && report.header.incidentName) incidentName = report.header.incidentName;
				if (!dateFrom && report.header.dateFrom) dateFrom = report.header.dateFrom.substring(0, 16);
				if (!dateTo && report.header.dateTo) dateTo = report.header.dateTo.substring(0, 16);
				if (!operatorName && report.header.operatorName) operatorName = report.header.operatorName;
				if (!stationId && report.header.stationId) stationId = report.header.stationId;
			}
		} catch (e) {
			error = 'Failed to load ICS-309 data';
			console.error(e);
		} finally {
			loading = false;
		}
	}

	function handleRefresh() {
		loadReport();
	}

	function handlePrint() {
		window.print();
	}

	function handleExportCSV() {
		const params: Record<string, string> = {};
		if ($ics309NetId) params.netId = $ics309NetId;
		if (incidentName) params.incidentName = incidentName;
		if (dateFrom) params.from = new Date(dateFrom).toISOString();
		if (dateTo) params.to = new Date(dateTo).toISOString();
		if (operatorName) params.operatorName = operatorName;
		if (stationId) params.stationId = stationId;

		window.open(api.ics309ExportUrl(params), '_blank');
	}

	function formatDateTime(iso: string): string {
		try {
			const d = new Date(iso);
			return d.toLocaleString('en-US', {
				month: '2-digit', day: '2-digit', year: 'numeric',
				hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false
			});
		} catch {
			return iso;
		}
	}
</script>

<!-- Screen view -->
<div class="ics309-panel" class:printing={false}>
	<div class="panel-header no-print">
		<div class="header-left">
			<button class="back-btn" onclick={() => closePanel()} title="Back">
				<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
					<path d="M10 12L6 8l4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
			</button>
			<h2>ICS-309 Communications Log</h2>
		</div>
		<div class="header-actions">
			<button class="action-btn" onclick={handleRefresh} title="Refresh">
				<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
					<path d="M2 8a6 6 0 0 1 10.5-3.9M14 8a6 6 0 0 1-10.5 3.9" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
					<path d="M12 1v3.5h-3.5M4 15v-3.5h3.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
				</svg>
			</button>
			<button class="action-btn" onclick={handleExportCSV} title="Export CSV">CSV</button>
			<button class="action-btn primary" onclick={handlePrint} title="Print / Save PDF">Print</button>
		</div>
	</div>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if error}
		<div class="error-msg">{error}</div>
	{:else}
		<!-- Editable Header Form (screen only) -->
		<div class="header-form no-print">
			<div class="form-row">
				<div class="form-group wide">
					<label for="ics-incident">Incident Name</label>
					<input id="ics-incident" type="text" bind:value={incidentName} placeholder="Incident or Event Name" />
				</div>
			</div>
			<div class="form-row">
				<div class="form-group">
					<label for="ics-from">Op. Period From</label>
					<input id="ics-from" type="datetime-local" bind:value={dateFrom} />
				</div>
				<div class="form-group">
					<label for="ics-to">Op. Period To</label>
					<input id="ics-to" type="datetime-local" bind:value={dateTo} />
				</div>
			</div>
			<div class="form-row">
				<div class="form-group">
					<label for="ics-operator">Radio Operator</label>
					<input id="ics-operator" type="text" bind:value={operatorName} placeholder="Operator Name" />
				</div>
				<div class="form-group">
					<label for="ics-station">Station ID</label>
					<input id="ics-station" type="text" bind:value={stationId} placeholder="Callsign" />
				</div>
			</div>
		</div>

		<!-- Print-optimized ICS-309 form -->
		<div class="ics309-form">
			<div class="form-title print-only">ICS 309 - Communications Log</div>

			<div class="form-header print-only">
				<div class="header-field">
					<span class="field-label">1. Incident Name:</span>
					<span class="field-value">{incidentName || '—'}</span>
				</div>
				<div class="header-row-2">
					<div class="header-field">
						<span class="field-label">2. Operational Period From:</span>
						<span class="field-value">{dateFrom ? formatDateTime(dateFrom) : '—'}</span>
					</div>
					<div class="header-field">
						<span class="field-label">To:</span>
						<span class="field-value">{dateTo ? formatDateTime(dateTo) : '—'}</span>
					</div>
				</div>
				<div class="header-row-2">
					<div class="header-field">
						<span class="field-label">3. Radio Operator Name:</span>
						<span class="field-value">{operatorName || '—'}</span>
					</div>
					<div class="header-field">
						<span class="field-label">4. Station ID:</span>
						<span class="field-value">{stationId || '—'}</span>
					</div>
				</div>
			</div>

			<!-- Communications Log Table -->
			<table class="log-table">
				<thead>
					<tr>
						<th class="col-num">#</th>
						<th class="col-time">Date/Time</th>
						<th class="col-from">From</th>
						<th class="col-to">To</th>
						<th class="col-subject">Subject / Message</th>
						<th class="col-method">Method</th>
					</tr>
				</thead>
				<tbody>
					{#if report && report.rows.length > 0}
						{#each report.rows as row, i}
							<tr>
								<td class="col-num">{i + 1}</td>
								<td class="col-time mono">{formatDateTime(row.dateTime)}</td>
								<td class="col-from mono">{row.from}</td>
								<td class="col-to mono">{row.to}</td>
								<td class="col-subject">{row.subject}</td>
								<td class="col-method">{row.method}</td>
							</tr>
						{/each}
					{:else}
						<tr>
							<td colspan="6" class="empty-row">No communications in selected period</td>
						</tr>
					{/if}
				</tbody>
			</table>

			<div class="form-footer print-only">
				<div class="footer-field">
					<span class="field-label">5. Prepared by:</span>
					<span class="field-value">{operatorName || '_______________'}</span>
				</div>
				<div class="footer-field">
					<span class="field-label">Date/Time:</span>
					<span class="field-value">{formatDateTime(new Date().toISOString())}</span>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.ics309-panel {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow-y: auto;
	}

	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		flex-shrink: 0;
	}

	.header-left {
		display: flex;
		align-items: center;
		gap: var(--space-sm);
	}

	.panel-header h2 {
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.back-btn {
		background: none;
		border: none;
		color: var(--color-text-muted);
		cursor: pointer;
		padding: 4px;
		border-radius: var(--radius-sm);
		display: flex;
		align-items: center;
	}
	.back-btn:hover {
		background: var(--color-primary);
		color: var(--color-text);
	}

	.header-actions {
		display: flex;
		gap: var(--space-xs);
	}

	.action-btn {
		background: var(--color-primary);
		border: 1px solid color-mix(in srgb, var(--color-primary) 60%, var(--color-text) 40%);
		color: var(--color-text);
		padding: 4px 10px;
		border-radius: var(--radius-sm);
		cursor: pointer;
		font-size: 0.75rem;
		font-weight: 600;
	}
	.action-btn:hover {
		background: color-mix(in srgb, var(--color-primary) 70%, var(--color-text) 30%);
	}
	.action-btn.primary {
		background: var(--color-accent);
		border-color: var(--color-accent);
	}
	.action-btn.primary:hover {
		background: color-mix(in srgb, var(--color-accent) 80%, white 20%);
	}

	.loading {
		padding: var(--space-xl);
		text-align: center;
		color: var(--color-text-muted);
	}

	.error-msg {
		padding: var(--space-md);
		color: var(--color-error);
		text-align: center;
	}

	/* Header form (screen only) */
	.header-form {
		padding: var(--space-sm) var(--space-md);
		border-bottom: 1px solid var(--color-primary);
		display: flex;
		flex-direction: column;
		gap: var(--space-xs);
	}

	.form-row {
		display: flex;
		gap: var(--space-sm);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: 2px;
		flex: 1;
	}

	.form-group.wide {
		flex: 1;
	}

	.form-group label {
		font-size: 0.65rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.form-group input {
		background: var(--color-surface);
		border: 1px solid var(--color-primary);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: 4px 8px;
		font-size: 0.8rem;
	}
	.form-group input:focus {
		outline: none;
		border-color: var(--color-accent);
	}

	/* ICS-309 Form */
	.ics309-form {
		padding: var(--space-sm) var(--space-md);
		flex: 1;
	}

	/* Log Table */
	.log-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.78rem;
	}

	.log-table th {
		background: var(--color-primary);
		color: var(--color-text);
		padding: 6px 8px;
		text-align: left;
		font-weight: 600;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.3px;
		border: 1px solid color-mix(in srgb, var(--color-primary) 60%, var(--color-text) 40%);
		position: sticky;
		top: 0;
	}

	.log-table td {
		padding: 5px 8px;
		border: 1px solid color-mix(in srgb, var(--color-primary) 80%, transparent);
		color: var(--color-text);
		vertical-align: top;
	}

	.log-table tbody tr:nth-child(even) {
		background: color-mix(in srgb, var(--color-surface) 60%, var(--color-bg) 40%);
	}

	.log-table tbody tr:hover {
		background: color-mix(in srgb, var(--color-primary) 40%, transparent);
	}

	.mono {
		font-family: monospace;
		font-size: 0.75rem;
	}

	.col-num { width: 32px; text-align: center; }
	.col-time { width: 140px; white-space: nowrap; }
	.col-from { width: 80px; }
	.col-to { width: 80px; }
	.col-subject { min-width: 120px; }
	.col-method { width: 50px; text-align: center; }

	.empty-row {
		text-align: center;
		color: var(--color-text-muted);
		padding: var(--space-xl) !important;
		font-style: italic;
	}

	/* Print-only elements are hidden on screen */
	.print-only {
		display: none;
	}

	/* Print styles */
	@media print {
		.no-print {
			display: none !important;
		}

		.print-only {
			display: block;
		}

		.ics309-panel {
			overflow: visible;
			height: auto;
			background: white;
			color: black;
		}

		.ics309-form {
			padding: 0;
		}

		.form-title {
			text-align: center;
			font-size: 16pt;
			font-weight: bold;
			margin-bottom: 12pt;
			padding-bottom: 6pt;
			border-bottom: 2px solid black;
		}

		.form-header {
			margin-bottom: 12pt;
		}

		.header-field {
			margin-bottom: 4pt;
		}

		.header-row-2 {
			display: flex;
			gap: 24pt;
		}

		.field-label {
			font-weight: bold;
			font-size: 10pt;
		}

		.field-value {
			font-size: 10pt;
			margin-left: 4pt;
		}

		.log-table {
			font-size: 9pt;
		}

		.log-table th {
			background: #e5e5e5 !important;
			color: black !important;
			border: 1px solid black;
			-webkit-print-color-adjust: exact;
			print-color-adjust: exact;
		}

		.log-table td {
			border: 1px solid #666;
			color: black;
		}

		.log-table tbody tr:nth-child(even) {
			background: #f5f5f5 !important;
			-webkit-print-color-adjust: exact;
			print-color-adjust: exact;
		}

		.form-footer {
			margin-top: 24pt;
			padding-top: 12pt;
			border-top: 1px solid #666;
			display: flex;
			gap: 48pt;
		}

		.footer-field {
			display: flex;
			gap: 4pt;
		}

		.mono {
			font-family: 'Courier New', monospace;
		}
	}
</style>
