import { useCallback, useReducer, useEffect, useState } from "react";
import { Composer } from "../components/Composer";
import { MessageList } from "../components/MessageList";
import { useTimeline, useSubmitTurn } from "../api/queries";
import {
  useSessionEvents,
  reduceChat,
  initialState,
  timelineToMessages,
  type ChatState,
} from "../api/events";
import type { SSEEvent } from "../api/types";

interface ChatPageProps {
  sessionId: string;
  onConnectionChange?: (connection: ChatState["connection"]) => void;
}

export function ChatPage({ sessionId, onConnectionChange }: ChatPageProps) {
  const [state, dispatch] = useReducer(reduceChat, initialState);
  const { data: timeline } = useTimeline(sessionId || null);
  const [timelineReady, setTimelineReady] = useState(false);
  const submitTurn = useSubmitTurn(sessionId);

  // Initialize from timeline on session load
  useEffect(() => {
    setTimelineReady(false);
  }, [sessionId]);

  useEffect(() => {
    if (timeline?.blocks) {
      dispatch({
        type: "init",
        messages: timelineToMessages(timeline.blocks),
        latestSeq: timeline.latest_seq,
      });
      setTimelineReady(true);
    }
  }, [timeline]);

  // SSE events
  const handleEvent = useCallback((event: SSEEvent) => {
    dispatch({ type: "event", event });
  }, []);

  const handleConnectionChange = useCallback(
    (value: ChatState["connection"]) => {
      dispatch({ type: "connection", value });
      onConnectionChange?.(value);
    },
    [onConnectionChange],
  );

  useSessionEvents(
    sessionId,
    state.latestSeq,
    handleEvent,
    handleConnectionChange,
    timelineReady,
  );

  async function handleSend(message: string) {
    if (submitTurn.isPending || !sessionId) return;
    // Optimistically mark user message
    dispatch({
      type: "appendUserMessage",
      id: `opt_${Date.now()}`,
      seq: state.latestSeq + 1,
      text: message,
    });
    try {
      await submitTurn.mutateAsync(message);
    } catch (err) {
      console.error("Failed to send message:", err);
      throw err;
    }
  }

  return (
    <div className="flex flex-col h-full">
      <MessageList messages={state.messages} connection={state.connection} onSuggestionClick={handleSend} suggestionsDisabled={submitTurn.isPending || !sessionId} />
      <Composer onSend={handleSend} disabled={submitTurn.isPending || !sessionId} />
    </div>
  );
}
