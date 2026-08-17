import { writable, derived } from 'svelte/store';
import type { Conversation, Message } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';
import { showToast } from './toast';
import { transports } from './transports';

export const conversations = writable<Map<string, Conversation>>(new Map());
export const conversationList = derived(conversations, ($convos) =>
	Array.from($convos.values()).sort(
		(a, b) => new Date(b.lastActive).getTime() - new Date(a.lastActive).getTime()
	)
);

let initialized = false;

/** Callsigns with a mark-read request in flight, to suppress duplicate POSTs. */
const pendingReads = new Set<string>();
/**
 * Inbound messages that landed while a mark-read was in flight, per callsign.
 * The server's marker is stamped mid-request, so anything that arrives after
 * we start is still unread and must survive the optimistic zeroing.
 */
const inflightArrivals = new Map<string, number>();
/**
 * Backoff state for failed mark-read requests, per callsign.
 *
 * Restoring the badge re-triggers the effect that issued the request, so a
 * failing server would otherwise be hammered in a tight loop. This throttles
 * retries instead of blocking them permanently: a transient failure (LTE blip,
 * proxy 502, token refresh) must never leave a conversation stuck at "unread
 * forever", which is the very bug this feature fixes.
 */
const readBackoff = new Map<string, { failures: number; retryAt: number }>();

const READ_RETRY_BASE_MS = 2000;
const READ_RETRY_MAX_MS = 30000;

export function initMessageStore(): void {
	if (initialized) return;
	initialized = true;

	// Load initial conversations
	api.conversations().then((list) => {
		conversations.set(new Map(list.map((c) => [c.callsign, c])));
	}).catch(() => {});

	// Listen for message events
	wsClient.on('message_received', (msg) => {
		const m = msg.message as Message;
		if (!m) return;
		addMessageToConversation(m);
	});

	wsClient.on('message_sent', (msg) => {
		const m = msg.message as Message;
		if (!m) return;
		addMessageToConversation(m);
	});

	wsClient.on('message_acked', (msg) => {
		const m = msg.message as Message;
		if (!m) return;
		updateMessageInConversation(m);
	});

	wsClient.on('message_failed', (msg) => {
		const m = msg.message as Message;
		if (!m) return;
		updateMessageInConversation(m);
	});

	// Read state is shared server-side: when any operator opens a thread, every
	// connected client clears the badge without a manual refresh.
	//
	// NOTE: the payload key is `conversation`, not `message` — the `message` key
	// is always present on this event but carries only zero values. The
	// conversation is PARTIALLY filled (`messages` is null, `lastActive` is the
	// zero time), so merge fields and never assign `.messages` from it.
	wsClient.on('conversation_read', (msg) => {
		const c = msg.conversation as Partial<Conversation> | undefined;
		if (!c?.callsign) return;
		const callsign = c.callsign;
		conversations.update((map) => {
			const convo = map.get(callsign);
			if (!convo) return map;
			// The event means "read up to this marker", and the server emits it
			// in the same order it appends messages: anything the marker does not
			// cover arrives as its own message_received after this, re-raising the
			// count. So the count here is simply zero — the client never
			// re-derives unread from timestamps, which it cannot do faithfully
			// (Date floors to milliseconds, the server compares nanoseconds).
			if (convo.unreadCount === 0 && convo.lastReadAt === c.lastReadAt) {
				return map;
			}
			map.set(callsign, { ...convo, unreadCount: 0, lastReadAt: c.lastReadAt });
			return new Map(map);
		});
	});
}

function addMessageToConversation(m: Message): void {
	const remote = m.inbound ? m.from : m.to;
	// New traffic is a genuine state change — let a backed-off mark-read try
	// again immediately rather than waiting out the window.
	readBackoff.delete(remote);
	// A message that lands mid-request is not covered by the badge we are about
	// to clear; remember it so the reconcile step can put the badge back.
	if (m.inbound && pendingReads.has(remote)) {
		inflightArrivals.set(remote, (inflightArrivals.get(remote) ?? 0) + 1);
	}
	conversations.update((map) => {
		const existing = map.get(remote);
		const msgs = existing?.messages ?? [];
		// Avoid duplicates
		if (msgs.find((msg) => msg.id === m.id)) {
			return map;
		}
		const newMsgs = [...msgs, m].sort(
			(a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
		);
		// Spread the existing conversation first — rebuilding it from a literal
		// silently drops claim and read state on every inbound packet.
		map.set(remote, {
			...(existing ?? { callsign: remote, messages: [], unreadCount: 0, lastActive: m.timestamp }),
			callsign: remote,
			messages: newMsgs,
			unreadCount: (existing?.unreadCount ?? 0) + (m.inbound ? 1 : 0),
			lastActive: m.timestamp
		});
		return new Map(map);
	});
}

function updateMessageInConversation(m: Message): void {
	const remote = m.inbound ? m.from : m.to;
	conversations.update((map) => {
		const convo = map.get(remote);
		if (!convo) return map;
		const newMsgs = convo.messages.map((msg) => msg.id === m.id ? m : msg);
		map.set(remote, {
			...convo,
			messages: newMsgs
		});
		return new Map(map);
	});
}

export async function sendMessage(to: string, body: string, path?: string): Promise<Message> {
	const msg = await api.sendMessage(to, body, path);
	addMessageToConversation(msg);
	// TX counts live on transport status; don't wait for the 5s ticker.
	api.transports().then((list) => transports.set(list)).catch(() => {});
	return msg;
}

export async function loadMessages(callsign: string): Promise<void> {
	const msgs = await api.messages(callsign);
	// Deduplicate by ID to prevent Svelte each_key_duplicate errors from legacy data
	const seen = new Set<string>();
	const unique = msgs.filter((m) => {
		if (seen.has(m.id)) return false;
		seen.add(m.id);
		return true;
	});
	conversations.update((map) => {
		const convo = map.get(callsign) ?? {
			callsign,
			messages: [],
			unreadCount: 0,
			lastActive: new Date().toISOString()
		};
		// unreadCount is server-derived now (see markConversationRead). Zeroing
		// it here would race the mark-read POST and could mask a genuine unread.
		map.set(callsign, { ...convo, messages: unique });
		return new Map(map);
	});
}

/**
 * Clears the unread badge for a conversation, server-side and persistently.
 *
 * Optimistic: the badge drops immediately so there is no visible bounce while
 * the request is in flight. On failure the previous count is restored, the user
 * is told, and the next attempt is delayed — never abandoned.
 */
export async function markConversationRead(callsign: string): Promise<void> {
	// Bulletins have no read state — the server rejects them with a 400, and
	// BulletinPanel materializes phantom BLN*/ANN* conversations client-side.
	if (!callsign || /^(BLN|ANN)/i.test(callsign)) return;
	if (pendingReads.has(callsign)) return;
	const backoff = readBackoff.get(callsign);
	if (backoff && Date.now() < backoff.retryAt) return;

	pendingReads.add(callsign);
	inflightArrivals.delete(callsign);

	let prevUnread = 0;
	conversations.update((map) => {
		const convo = map.get(callsign);
		if (!convo || convo.unreadCount === 0) return map;
		prevUnread = convo.unreadCount;
		map.set(callsign, { ...convo, unreadCount: 0 });
		return new Map(map);
	});

	try {
		const res = await api.markConversationRead(callsign);
		readBackoff.delete(callsign);
		// Anything that landed mid-request is not covered by the marker the
		// server just stamped, so it stays unread. Counting arrivals rather than
		// re-deriving from timestamps keeps the client from ever disagreeing
		// with the server about sub-millisecond ordering.
		const arrived = inflightArrivals.get(callsign) ?? 0;
		conversations.update((map) => {
			const convo = map.get(callsign);
			if (!convo) return map;
			map.set(callsign, {
				...convo,
				unreadCount: arrived,
				lastReadAt: res.lastReadAt ?? undefined
			});
			return new Map(map);
		});
	} catch {
		const failures = (readBackoff.get(callsign)?.failures ?? 0) + 1;
		const delay = Math.min(READ_RETRY_MAX_MS, READ_RETRY_BASE_MS * 2 ** (failures - 1));
		readBackoff.set(callsign, { failures, retryAt: Date.now() + delay });
		if (prevUnread > 0) {
			conversations.update((map) => {
				const convo = map.get(callsign);
				if (!convo) return map;
				map.set(callsign, { ...convo, unreadCount: convo.unreadCount + prevUnread });
				return new Map(map);
			});
		}
		// Tell the operator: the badge coming back on its own would otherwise
		// look like a new message. The next attempt happens when they next look
		// at the thread or new traffic arrives — throttled, never blocked.
		showToast(`Couldn't mark ${callsign} read`, 'error');
	} finally {
		pendingReads.delete(callsign);
		inflightArrivals.delete(callsign);
	}
}

export async function claimConversation(callsign: string, userId: string, userName: string): Promise<void> {
	await api.claimConversation(callsign, userId, userName);
	conversations.update((map) => {
		const convo = map.get(callsign);
		if (convo) {
			map.set(callsign, { ...convo, claimedBy: userId, claimedName: userName });
		}
		return new Map(map);
	});
}

export async function unclaimConversation(callsign: string): Promise<void> {
	await api.unclaimConversation(callsign);
	conversations.update((map) => {
		const convo = map.get(callsign);
		if (convo) {
			map.set(callsign, { ...convo, claimedBy: undefined, claimedName: undefined });
		}
		return new Map(map);
	});
}
