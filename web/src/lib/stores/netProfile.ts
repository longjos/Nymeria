// Net-profile registry + panel gating (WP7, part 2). The per-active-net
// profile view itself (NetProfileView, the effective priority ladder,
// isRideNet) already lives in stores/ride.ts as `rideProfile`/`rideLadder`/
// `rideMode` — loaded once per net by initRideStore() and kept live over the
// same WebSocket connection. This file does NOT duplicate that fetch; it
// only adds:
//   - the net-profile REGISTRY (all profiles, for the create-net form),
//     which is not net-scoped and rideProfile has no reason to carry;
//   - panel-gating derived stores built from rideProfile's panel ids;
//   - ride-config save, which refreshes the shared rideProfile afterward so
//     every consumer (RideConfigSheet, the ladder editor, the strip) sees
//     the new effective ladder without a reload.
import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api';
import type { NetProfile, NetRideConfig } from '$lib/types';
import { rideProfile, rideMode, rideLadder } from './ride';
import { wxIsNcs } from './wxAlerts';
import { hasPanel, visibleModesFor } from '$lib/profilePanels';

/** GET /api/net-profiles — the whole registry, for the create-net form's
 * Profile select. Loaded lazily (once) when that form opens. */
export const netProfiles = writable<NetProfile[]>([]);

let profilesRequested = false;

export async function loadNetProfiles(): Promise<void> {
	if (profilesRequested && get(netProfiles).length > 0) return;
	profilesRequested = true;
	try {
		netProfiles.set(await api.netProfiles());
	} catch {
		profilesRequested = false; // allow a retry on the next open
	}
}

/** Domain-neutral re-exports: the ride-mode identity and priority ladder
 * live in stores/ride.ts (loaded once per active net); course/net-settings
 * surfaces read them from here so they never import ride.ts's SAG/medical
 * internals just to ask "is this a ride net". */
export const isRideNet = rideMode;
export const effectiveTiers = rideLadder;
export const isNetNcs = wxIsNcs;
export const rideConfig = derived(rideProfile, (p) => p?.rideConfig ?? null);

export const profilePanelIds = derived(rideProfile, (p) => p?.profile.panels ?? null);
export const visiblePanelModes = derived(profilePanelIds, visibleModesFor);
export const profileHasPanel = derived(profilePanelIds, (ids) => (id: string) => hasPanel(ids, id));

/**
 * PUT /nets/{id}/ride-config, then refetches the shared rideProfile view.
 * Throws ApiError on failure (the 400 validation message from
 * netprofile.ValidateRideConfig, e.g. "tier ranks must be 1..5 with no gaps
 * or repeats") for the caller to show inline.
 */
export async function saveRideConfig(netId: string, cfg: NetRideConfig): Promise<NetRideConfig> {
	const saved = await api.updateRideConfig(netId, cfg);
	try {
		rideProfile.set(await api.netProfile(netId));
	} catch {
		// the save itself already succeeded; the view will catch up on the
		// next net-identity reload or net_updated event.
	}
	return saved;
}

/** Draft-only (409 profile_mismatch/not_draft otherwise) — see
 * handlePutNetProfile's doc comment. Exposed for completeness; the normal
 * path is choosing the profile at net creation (api.createNet). */
export async function setNetProfile(netId: string, profile: string) {
	const view = await api.setNetProfile(netId, profile);
	rideProfile.set(view);
	return view;
}
