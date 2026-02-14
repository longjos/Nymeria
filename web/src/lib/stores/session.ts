import { writable, derived, get } from 'svelte/store';
import type { SessionUser, Role } from '$lib/types';
import { api, setAuthToken, loadSavedToken } from '$lib/api';

export const currentUser = writable<SessionUser | null>(null);
export const needsSetup = writable<boolean>(false);
export const pendingRequests = writable<SessionUser[]>([]);

export const isLoggedIn = derived(currentUser, ($u) => $u !== null);
export const userRole = derived(currentUser, ($u) => $u?.role ?? null);
export const userName = derived(currentUser, ($u) => $u?.name ?? null);
export const userStatus = derived(currentUser, ($u) => $u?.status ?? null);
export const isPending = derived(currentUser, ($u) => $u?.status === 'pending');
export const isDenied = derived(currentUser, ($u) => $u?.status === 'denied');
export const isApproved = derived(currentUser, ($u) => $u?.status === 'approved');

const ROLE_LEVELS: Record<Role, number> = {
	observer: 0,
	plotter: 1,
	operator: 2,
	admin: 3
};

/** Check if the current user has at least the given role level. */
export function hasRole(minRole: Role): boolean {
	const user = get(currentUser);
	if (!user || user.status !== 'approved') return false;
	return (ROLE_LEVELS[user.role] ?? -1) >= (ROLE_LEVELS[minRole] ?? 99);
}

/** Derived store version: true if user has at least the given role. */
export function canRole(minRole: Role) {
	return derived(currentUser, ($u) => {
		if (!$u || $u.status !== 'approved') return false;
		return (ROLE_LEVELS[$u.role] ?? -1) >= (ROLE_LEVELS[minRole] ?? 99);
	});
}

export const canPlot = canRole('plotter');
export const canOperate = canRole('operator');
export const canAdmin = canRole('admin');

/** Fetch server config and restore saved session if token exists. */
export async function initSession() {
	try {
		const cfg = await api.config();
		needsSetup.set(cfg.needsSetup);
	} catch {
		needsSetup.set(false);
	}

	// Restore session from saved token
	const saved = loadSavedToken();
	if (saved) {
		setAuthToken(saved);
		try {
			const user = await api.session();
			currentUser.set(user);
		} catch {
			// Token expired or invalid — clear it
			setAuthToken(null);
		}
	}
}

/** Log in with a display name. Sends saved token for reconnection. */
export async function login(name: string): Promise<SessionUser> {
	const savedToken = loadSavedToken() ?? undefined;
	const user = await api.login(name, savedToken);
	setAuthToken(user.token);
	currentUser.set(user);
	return user;
}

/** Log out the current user. */
export async function logout() {
	try {
		await api.logout();
	} catch {
		// server may already have removed the session
	}
	setAuthToken(null);
	currentUser.set(null);
}

/** Load pending access requests (for admin UI). */
export async function loadPendingRequests() {
	try {
		const pending = await api.getPending();
		pendingRequests.set(pending);
	} catch {
		pendingRequests.set([]);
	}
}

/** Handle session WebSocket events — call from WS message handlers. */
export function handleSessionEvent(msg: Record<string, unknown>) {
	const type = msg.type as string;
	const user = msg.user as SessionUser | undefined;

	if (!user) return;

	switch (type) {
		case 'access_approved': {
			const cur = get(currentUser);
			if (cur && cur.id === user.id) {
				currentUser.set({ ...cur, status: 'approved', role: user.role });
			}
			// Remove from pending list
			pendingRequests.update((list) => list.filter((u) => u.id !== user.id));
			break;
		}
		case 'access_denied': {
			const cur = get(currentUser);
			if (cur && cur.id === user.id) {
				currentUser.set({ ...cur, status: 'denied' });
			}
			// Remove from pending list
			pendingRequests.update((list) => list.filter((u) => u.id !== user.id));
			break;
		}
		case 'access_request': {
			// Add to pending requests (for admin notification)
			pendingRequests.update((list) => {
				if (list.some((u) => u.id === user.id)) return list;
				return [...list, user];
			});
			break;
		}
	}
}
