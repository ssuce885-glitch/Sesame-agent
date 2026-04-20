import { useCallback, useReducer, useEffect } from "react";
import { Composer } from "../components/Composer";
import { MessageList } from "../components/MessageList";
import { useTimeline, useSubmitMessage } from "../api/queries";
import {
  useSessionEvents,
  reduceChat,
  initialState,
  timelineToMessages,
  type ChatState,
  type ChatAction,
} from "../api/events";

interface ChatPageProps {
  sessionId: string;
}

export function ChatPage({ sessionId }: ChatPageProps) {
  const [state, dispatch] = useReducer(reduceChat, initialState);
  const { data: timeline, isLoading } = useTimeline(sessionId);
  const submitMessage = useSubmitMessage(sessionId);

  // Initialize from timeline on session load
  useEffect(() => {
    if (timeline?.blocks) {
      dispatch({
        type: "init",
        messages: timelineToMessages(timeline.blocks),
        latestSeq: timeline.latest_seq,
      });
    }
  }, [timeline]);

  // SSE events
  const handleEvent = useCallback((event: import("../api/types").ServerEvent) => {
    dispatch({ type: "event", event });
  }, []);

  const handleConnectionChange = useCallback(
    (value: ChatState["connection"]) => {
      dispatch({ type: "connection", value });
    },
    [],
  );

  const { reconnect } = useSessionEvents(
    sessionId,
    state.latestSeq,
    handleEvent,
    handleConnectionChange,
  );

  async function handleSend(message: string) {
    // Optimistically mark user message
    dispatch({
      type: "event",
      event: {
        id: `opt_${Date.now()}`,
        seq: state.latestSeq + 1,
        session_id: sessionId,
        turn_id: undefined,
        type: "user_message",
        time: new Date().toISOString(),
        payload: { text: message },
      },
    } as ChatAction);
    try {
      await submitMessage.mutateAsync(message);
    } catch (err) {
      console.error("Failed to send message:", err);
    }
  }

  return (
    <div className="flex flex-col h-full">
      <MessageList messages={state.messages} connection={state.connection} />
      <Composer onSend={handleSend} disabled={submitMessage.isPending} />
    </div>
  );
}
