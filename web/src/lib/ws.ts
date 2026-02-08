type MessageHandler = (data: unknown) => void;

export class WSClient {
	private ws: WebSocket | null = null;
	private handlers: Map<string, MessageHandler[]> = new Map();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

	connect(url?: string): void {
		const wsUrl = url ?? `ws://${window.location.host}/ws`;
		this.ws = new WebSocket(wsUrl);

		this.ws.onmessage = (event) => {
			try {
				const msg = JSON.parse(event.data);
				const handlers = this.handlers.get(msg.type) ?? [];
				for (const handler of handlers) {
					handler(msg.data);
				}
			} catch {
				console.warn('Invalid WebSocket message:', event.data);
			}
		};

		this.ws.onclose = () => {
			this.reconnectTimer = setTimeout(() => this.connect(url), 3000);
		};

		this.ws.onerror = () => {
			this.ws?.close();
		};
	}

	on(type: string, handler: MessageHandler): void {
		const list = this.handlers.get(type) ?? [];
		list.push(handler);
		this.handlers.set(type, list);
	}

	send(type: string, data: unknown): void {
		this.ws?.send(JSON.stringify({ type, data }));
	}

	disconnect(): void {
		if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
		this.ws?.close();
	}
}
