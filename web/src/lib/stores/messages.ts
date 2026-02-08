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
		const convo = map.get(remote) ?? {
			callsign: remote,
			messages: [],
			unreadCount: 0,
			lastActive: m.timestamp
		};
		// Avoid duplicates
		if (!convo.messages.find((msg) => msg.id === m.id)) {
			convo.messages.push(m);
			convo.messages.sort(
				(a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
			);
		}
		if (m.inbound) convo.unreadCount++;
		convo.lastActive = m.timestamp;
		map.set(remote, convo);
		return new Map(map);
	});
}

function updateMessageInConversation(m: Message): void {
	const remote = m.inbound ? m.from : m.to;
	conversations.update((map) => {
		const convo = map.get(remote);
		if (!convo) return map;
		const idx = convo.messages.findIndex((msg) => msg.id === m.id);
		if (idx >= 0) convo.messages[idx] = m;
		map.set(remote, convo);
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
	conversations.update((map) => {
		const convo = map.get(callsign) ?? {
			callsign,
			messages: [],
			unreadCount: 0,
			lastActive: new Date().toISOString()
		};
		convo.messages = msgs;
		convo.unreadCount = 0;
		map.set(callsign, convo);
		return new Map(map);
	});
}
