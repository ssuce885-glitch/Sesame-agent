import type { 服务端事件 } from "./api";

export const LIVENESS_TIMEOUT_MS = 45_000;

const RECONNECT_BASE_DELAY_MS = 1_000;
const RECONNECT_MAX_DELAY_MS = 15_000;

export type EventSourceLike = {
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent<string>) => void) | null;
  onerror: ((event: Event) => void) | null;
  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void;
  close(): void;
};

export type StreamScheduler = {
  setTimeout: typeof globalThis.setTimeout;
  clearTimeout: typeof globalThis.clearTimeout;
};

type ConnectionState = "connecting" | "open" | "reconnecting";

type StartConversationEventStreamOptions = {
  sessionId: string;
  latestSeqRef: { current: number };
  openEventSource: (sessionId: string, after: number) => EventSourceLike;
  onConnectionChange: (value: ConnectionState) => void;
  onBusinessEvent: (event: 服务端事件) => void;
  onRecoverFromSnapshot: () => Promise<number>;
  onTerminalEvent?: (eventType: string) => void;
  scheduler?: StreamScheduler;
  random?: () => number;
};

type KeepalivePayload = {
  session_id: string;
  latest_seq: number;
  time: string;
};

export function classifyBusinessEventSeq(lastAppliedSeq: number, nextSeq: number) {
  if (nextSeq <= lastAppliedSeq) {
    return "ignore" as const;
  }
  if (nextSeq === lastAppliedSeq + 1) {
    return "apply" as const;
  }
  return "gap" as const;
}

export function getReconnectDelayMs(attempt: number, random: () => number = Math.random) {
  const baseDelay = Math.min(RECONNECT_BASE_DELAY_MS * 2 ** Math.max(0, attempt - 1), RECONNECT_MAX_DELAY_MS);
  const jitterFactor = 0.75 + random() * 0.5;
  return Math.round(baseDelay * jitterFactor);
}

export function startConversationEventStream(options: StartConversationEventStreamOptions) {
  const scheduler = options.scheduler ?? globalThis;

  let disposed = false;
  let reconnectAttempts = 0;
  let source: EventSourceLike | null = null;
  let retryTimer: ReturnType<typeof globalThis.setTimeout> | null = null;
  let livenessTimer: ReturnType<typeof globalThis.setTimeout> | null = null;

  const clearRetryTimer = () => {
    if (retryTimer !== null) {
      scheduler.clearTimeout(retryTimer);
      retryTimer = null;
    }
  };

  const clearLivenessTimer = () => {
    if (livenessTimer !== null) {
      scheduler.clearTimeout(livenessTimer);
      livenessTimer = null;
    }
  };

  const closeSource = () => {
    clearLivenessTimer();
    source?.close();
    source = null;
  };

  const scheduleReconnect = () => {
    closeSource();
    if (disposed || retryTimer !== null) {
      return;
    }

    options.onConnectionChange("reconnecting");
    const delay = getReconnectDelayMs(++reconnectAttempts, options.random);
    retryTimer = scheduler.setTimeout(() => {
      retryTimer = null;
      connect();
    }, delay);
  };

  const recoverFromSnapshot = async () => {
    closeSource();
    clearRetryTimer();
    if (disposed) {
      return;
    }

    options.onConnectionChange("reconnecting");
    try {
      const latestSeq = await options.onRecoverFromSnapshot();
      if (disposed) {
        return;
      }
      options.latestSeqRef.current = latestSeq;
      reconnectAttempts = 0;
      connect();
    } catch {
      scheduleReconnect();
    }
  };

  const resetLiveness = () => {
    clearLivenessTimer();
    livenessTimer = scheduler.setTimeout(() => {
      livenessTimer = null;
      closeSource();
      scheduleReconnect();
    }, LIVENESS_TIMEOUT_MS);
  };

  const handleBusinessMessage = (message: MessageEvent<string>) => {
    resetLiveness();

    const event = JSON.parse(message.data) as 服务端事件;
    const decision = classifyBusinessEventSeq(options.latestSeqRef.current, event.seq);
    if (decision === "ignore") {
      return;
    }
    if (decision === "gap") {
      void recoverFromSnapshot();
      return;
    }

    options.latestSeqRef.current = event.seq;
    options.onBusinessEvent(event);
    if (event.type === "turn.completed" || event.type === "turn.failed") {
      options.onTerminalEvent?.(event.type);
    }
  };

  const handleKeepalive: EventListener = (event) => {
    try {
      const message = event as MessageEvent<string>;
      JSON.parse(message.data) as KeepalivePayload;
      resetLiveness();
    } catch {
      scheduleReconnect();
    }
  };

const BUSINESS_EVENT_TYPES = [
  "turn.started", "turn.completed", "turn.failed", "turn.usage",
  "assistant.started", "assistant.delta", "assistant.completed",
  "tool.started", "tool.completed",
  "system.notice", "context.compacted",
] as const;

  const connect = () => {
    if (disposed) {
      return;
    }

    options.onConnectionChange(options.latestSeqRef.current > 0 ? "reconnecting" : "connecting");
    source = options.openEventSource(options.sessionId, options.latestSeqRef.current);
    source.onopen = () => {
      reconnectAttempts = 0;
      options.onConnectionChange("open");
      resetLiveness();
    };
    for (const eventType of BUSINESS_EVENT_TYPES) {
      source.addEventListener(eventType, handleBusinessMessage);
    }
    source.onerror = () => {
      scheduleReconnect();
    };
    source.addEventListener("keepalive", handleKeepalive);
  };

  connect();

  return () => {
    disposed = true;
    clearRetryTimer();
    closeSource();
  };
}
