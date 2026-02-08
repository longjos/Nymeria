import { writable, derived } from 'svelte/store';
import type { Conversation, Message } from '$lib/types';
import { api } from '$lib/api';
import { wsClient } from './stations';

export const conversations = writable<Map<string, Conversation>>(new Map());
export const conversationList = derived(conversations, ($convos) =>
	Array.from($convos.values()).sort(
		(a, b) => new Date(b.lastActive).getTime() - new Date(a.lastActive).getTime()
	)
);

let initialized = false;

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
}

function addMessageToConversation(m: Message): void {
	const remote = m.inbound ? m.from : m.to;
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
		map.set(remote, {
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

export async function sendMessage(to: string, body: string): Promise<Message> {
	const msg = await api.sendMessage(to, body);
	addMessageToConversation(msg);
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
		convo.messages = unique;
		convo.unreadCount = 0;
		map.set(callsign, convo);
		return new Map(map);
	});
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
