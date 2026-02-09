import { writable, derived, get } from 'svelte/store';
import type { SessionUser, Role } from '$lib/types';
import { api, setAuthToken, loadSavedToken } from '$lib/api';

export const currentUser = writable<SessionUser | null>(null);
export const pinRequired = writable<boolean>(false);

export const isLoggedIn = derived(currentUser, ($u) => $u !== null);
export const userRole = derived(currentUser, ($u) => $u?.role ?? null);
export const userName = derived(currentUser, ($u) => $u?.name ?? null);

const ROLE_LEVELS: Record<Role, number> = {
	observer: 0,
	plotter: 1,
	operator: 2,
	admin: 3
};

/** Check if the current user has at least the given role level. */
export function hasRole(minRole: Role): boolean {
	const user = get(currentUser);
	if (!user) return false;
	return (ROLE_LEVELS[user.role] ?? -1) >= (ROLE_LEVELS[minRole] ?? 99);
}

/** Derived store version: true if user has at least the given role. */
export function canRole(minRole: Role) {
	return derived(currentUser, ($u) => {
		if (!$u) return false;
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
		pinRequired.set(cfg.pinRequired);
	} catch {
		pinRequired.set(false);
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

/** Log in with a display name and optional PIN. */
export async function login(name: string, pin?: string): Promise<SessionUser> {
	const user = await api.login(name, pin);
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
