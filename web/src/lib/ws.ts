type MessageHandler = (data: Record<string, unknown>) => void;

export class WSClient {
	private ws: WebSocket | null = null;
	private handlers: Map<string, MessageHandler[]> = new Map();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private _url: string | undefined;

	connect(url?: string): void {
		this._url = url;
		const wsUrl = url ?? `ws://${window.location.host}/ws`;
		this.ws = new WebSocket(wsUrl);

		this.ws.onmessage = (event) => {
			try {
				const msg = JSON.parse(event.data);
				const type = msg.type as string;
				if (!type) return;
				const handlers = this.handlers.get(type) ?? [];
				for (const handler of handlers) {
					handler(msg);
				}
				// Also fire wildcard handlers
				const wildcards = this.handlers.get('*') ?? [];
				for (const handler of wildcards) {
					handler(msg);
				}
			} catch {
				// ignore malformed messages
			}
		};

		this.ws.onclose = () => {
			this.reconnectTimer = setTimeout(() => this.connect(this._url), 3000);
		};

		this.ws.onerror = () => {
			this.ws?.close();
		};
	}

	on(type: string, handler: MessageHandler): () => void {
		const list = this.handlers.get(type) ?? [];
		list.push(handler);
		this.handlers.set(type, list);
		return () => {
			const idx = list.indexOf(handler);
			if (idx >= 0) list.splice(idx, 1);
		};
	}

	send(type: string, data: unknown): void {
		this.ws?.send(JSON.stringify({ type, data }));
	}

	disconnect(): void {
		if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
		this.reconnectTimer = null;
		this.ws?.close();
		this.ws = null;
	}
}
