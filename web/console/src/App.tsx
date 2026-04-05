import {
  startTransition,
  useEffect,
  useReducer,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";
import {
  Navigate,
  NavLink,
  Route,
  Routes,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import {
  创建会话,
  打开事件流,
  提交消息,
  获取会话列表,
  获取工作区,
  获取时间线,
  获取统计明细,
  获取统计时序,
  获取统计概览,
  选择会话,
  type 会话项,
  type 时间线块,
} from "./api";
import { 初始对话状态, 对话状态归并 } from "./chatState";

const TOOL_RESULT_INLINE_THRESHOLD = 300;
const TOOL_RESULT_SUMMARY_LIMIT = 80;
const CONTEXT_WARNING_THRESHOLD = 5000;
const CONTEXT_DANGER_THRESHOLD = 5800;
const INPUT_HISTORY_LIMIT = 50;

type 上下文告警 = {
  level: "warn" | "danger";
  text: string;
};

let 输入历史记录: string[] = [];

function 记录输入历史(value: string) {
  if (!value) {
    return;
  }
  const latestValue = 输入历史记录[输入历史记录.length - 1];
  if (latestValue === value) {
    return;
  }
  输入历史记录 = [...输入历史记录, value].slice(-INPUT_HISTORY_LIMIT);
}

function 生成工具结果摘要(resultPreview?: string) {
  if (!resultPreview) {
    return "";
  }
  if (resultPreview.length <= TOOL_RESULT_SUMMARY_LIMIT) {
    return resultPreview;
  }
  return `${resultPreview.slice(0, TOOL_RESULT_SUMMARY_LIMIT)}...`;
}

function 获取工具结果长度(resultPreview?: string) {
  if (!resultPreview) {
    return 0;
  }
  return new TextEncoder().encode(resultPreview).length;
}

export function mergeAdjacentAssistantBlocks(blocks: 时间线块[]) {
  return blocks.reduce<时间线块[]>((mergedBlocks, block) => {
    const previousBlock = mergedBlocks[mergedBlocks.length - 1];
    if (
      previousBlock?.kind === "assistant_output" &&
      block.kind === "assistant_output" &&
      previousBlock.turn_id &&
      block.turn_id &&
      previousBlock.turn_id === block.turn_id
    ) {
      mergedBlocks[mergedBlocks.length - 1] = {
        ...previousBlock,
        ...block,
        id: previousBlock.id,
        turn_id: previousBlock.turn_id,
        kind: "assistant_output",
        text: `${previousBlock.text ?? ""}${block.text ?? ""}`,
        status: block.status ?? previousBlock.status,
        usage: block.usage ?? previousBlock.usage,
      };
      return mergedBlocks;
    }

    mergedBlocks.push(block);
    return mergedBlocks;
  }, []);
}

export function getContextUsageWarning(blocks: 时间线块[]): 上下文告警 | null {
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index];
    if (block.kind !== "assistant_output" || !block.usage) {
      continue;
    }

    const totalTokens = block.usage.input_tokens + block.usage.cached_tokens;
    if (totalTokens >= CONTEXT_DANGER_THRESHOLD) {
      return {
        level: "danger",
        text: `上下文即将触发压缩（${totalTokens} tokens）`,
      };
    }
    if (totalTokens >= CONTEXT_WARNING_THRESHOLD) {
      return {
        level: "warn",
        text: `上下文已用 ${totalTokens} tokens，接近压缩阈值`,
      };
    }
    return null;
  }

  return null;
}

export function resetInputHistoryForTests() {
  输入历史记录 = [];
}

export default function 应用() {
  const [查询客户端] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={查询客户端}>
      <应用外壳>
        <Routes>
          <Route path="/" element={<Navigate to="/chat" replace />} />
          <Route path="/chat" element={<对话页面 />} />
          <Route path="/metrics" element={<统计页面 />} />
        </Routes>
      </应用外壳>
    </QueryClientProvider>
  );
}

function 应用外壳(props: { children: ReactNode }) {
  return (
    <div className="app-shell">
      <顶部导航 />
      <div className="page-shell">{props.children}</div>
    </div>
  );
}

function 顶部导航() {
  return (
    <header className="top-nav">
      <div className="brand-mark">
        <span className="brand-dot" />
        <div>
          <div className="brand-title">Agent Console</div>
          <div className="brand-subtitle">本机任务控制台</div>
        </div>
      </div>
      <nav className="nav-tabs" aria-label="主导航">
        <NavLink className={({ isActive }) => (isActive ? "nav-tab active" : "nav-tab")} to="/chat">
          对话
        </NavLink>
        <NavLink className={({ isActive }) => (isActive ? "nav-tab active" : "nav-tab")} to="/metrics">
          统计
        </NavLink>
      </nav>
    </header>
  );
}

function 对话页面() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [工作区路径, 设置工作区路径] = useState("");
  const [输入内容, 设置输入内容] = useState("");
  const [错误信息, 设置错误信息] = useState("");
  const [状态, 派发] = useReducer(对话状态归并, 初始对话状态);

  const 会话查询 = useQuery({
    queryKey: ["sessions"],
    queryFn: 获取会话列表,
  });

  const 当前会话ID =
    searchParams.get("session") ??
    会话查询.data?.selected_session_id ??
    会话查询.data?.sessions[0]?.id ??
    "";
  const 上下文Warning = getContextUsageWarning(状态.blocks);

  const 时间线查询 = useQuery({
    queryKey: ["timeline", 当前会话ID],
    queryFn: () => 获取时间线(当前会话ID),
    enabled: Boolean(当前会话ID),
  });

  const 工作区查询 = useQuery({
    queryKey: ["workspace", 当前会话ID],
    queryFn: () => 获取工作区(当前会话ID),
    enabled: Boolean(当前会话ID),
  });

  const 创建会话动作 = useMutation({
    mutationFn: 创建会话,
    onSuccess: async (session) => {
      await queryClient.invalidateQueries({ queryKey: ["sessions"] });
      startTransition(() => {
        setSearchParams((current) => {
          const next = new URLSearchParams(current);
          next.set("session", session.id);
          return next;
        });
      });
      设置工作区路径(session.workspace_root);
      设置错误信息("");
    },
  });

  const 选择会话动作 = useMutation({
    mutationFn: 选择会话,
    onSuccess: (_response, sessionId) => {
      startTransition(() => {
        setSearchParams((current) => {
          const next = new URLSearchParams(current);
          next.set("session", sessionId);
          return next;
        });
      });
    },
  });

  const 提交消息动作 = useMutation({
    mutationFn: ({ sessionId, message }: { sessionId: string; message: string }) => 提交消息(sessionId, message),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["sessions"] });
      await queryClient.invalidateQueries({ queryKey: ["metrics-overview"] });
      await queryClient.invalidateQueries({ queryKey: ["metrics-timeseries"] });
      await queryClient.invalidateQueries({ queryKey: ["metrics-turns"] });
    },
  });

  useEffect(() => {
    if (!searchParams.get("session") && 当前会话ID) {
      startTransition(() => {
        setSearchParams((current) => {
          const next = new URLSearchParams(current);
          next.set("session", 当前会话ID);
          return next;
        });
      });
    }
  }, [当前会话ID, searchParams, setSearchParams]);

  useEffect(() => {
    if (工作区查询.data?.workspace_root) {
      设置工作区路径(工作区查询.data.workspace_root);
    }
  }, [工作区查询.data?.workspace_root]);

  useEffect(() => {
    if (时间线查询.data) {
      派发({
        type: "snapshot",
        blocks: 时间线查询.data.blocks,
        latestSeq: 时间线查询.data.latest_seq,
      });
    } else if (!当前会话ID) {
      派发({
        type: "snapshot",
        blocks: [],
        latestSeq: 0,
      });
    }
  }, [当前会话ID, 时间线查询.data]);

  useEffect(() => {
    if (!当前会话ID || !时间线查询.data) {
      派发({ type: "connection", value: "idle" });
      return;
    }

    let disposed = false;
    let eventSource: EventSource | null = null;
    let retryTimer: number | null = null;
    let after = 时间线查询.data.latest_seq;

    const 连接事件流 = () => {
      if (disposed) {
        return;
      }
      派发({ type: "connection", value: after > 0 ? "reconnecting" : "connecting" });
      eventSource = 打开事件流(当前会话ID, after);
      eventSource.onopen = () => {
        派发({ type: "connection", value: "open" });
      };
      eventSource.onmessage = (message) => {
        const event = JSON.parse(message.data) as {
          seq: number;
        };
        after = event.seq;
        派发({ type: "event", event: JSON.parse(message.data) });

        const typedEvent = JSON.parse(message.data) as { type: string };
        if (typedEvent.type === "turn.completed" || typedEvent.type === "turn.failed") {
          void queryClient.invalidateQueries({ queryKey: ["timeline", 当前会话ID] });
          void queryClient.invalidateQueries({ queryKey: ["sessions"] });
          void queryClient.invalidateQueries({ queryKey: ["metrics-overview"] });
          void queryClient.invalidateQueries({ queryKey: ["metrics-timeseries"] });
          void queryClient.invalidateQueries({ queryKey: ["metrics-turns"] });
        }
      };
      eventSource.onerror = () => {
        eventSource?.close();
        if (disposed) {
          return;
        }
        派发({ type: "connection", value: "reconnecting" });
        retryTimer = window.setTimeout(连接事件流, 1500);
      };
    };

    连接事件流();
    return () => {
      disposed = true;
      eventSource?.close();
      if (retryTimer !== null) {
        window.clearTimeout(retryTimer);
      }
    };
  }, [当前会话ID, 时间线查询.data, queryClient]);

  async function 处理创建会话() {
    if (!工作区路径.trim()) {
      设置错误信息("请输入工作区路径后再新建会话。");
      return;
    }
    await 创建会话动作.mutateAsync(工作区路径.trim());
  }

  async function 处理发送消息() {
    const message = 输入内容.trim();
    if (!message) {
      return false;
    }

    let sessionId = 当前会话ID;
    if (!sessionId) {
      if (!工作区路径.trim()) {
        设置错误信息("请先输入工作区路径，或选择一个已有会话。");
        return false;
      }
      const session = await 创建会话动作.mutateAsync(工作区路径.trim());
      sessionId = session.id;
      await 选择会话动作.mutateAsync(session.id);
      navigate(`/chat?session=${session.id}`, { replace: true });
    }

    派发({ type: "optimistic-user", text: message });
    设置输入内容("");
    设置错误信息("");
    await 提交消息动作.mutateAsync({ sessionId, message });
    return true;
  }

  return (
    <div className="chat-layout">
      <会话列表栏
        selectedSessionId={当前会话ID}
        sessions={会话查询.data?.sessions ?? []}
        workspaceRoot={工作区路径}
        loading={会话查询.isLoading}
        onCreate={处理创建会话}
        onWorkspaceRootChange={设置工作区路径}
        onSelect={(sessionId) => {
          void 选择会话动作.mutateAsync(sessionId);
        }}
      />

      <section className="chat-main">
        <div className="chat-header">
          <div>
            <h1 className="section-title">对话任务流</h1>
            <p className="section-subtitle">
              用户消息、tool call 和最终输出都按时间顺序落在同一条流里。
            </p>
          </div>
          <div className="session-meta">
            <span className={`status-pill ${状态.connection}`}>{连接文案(状态.connection)}</span>
            {工作区查询.data?.model ? <span>{工作区查询.data.model}</span> : null}
            {工作区查询.data?.provider ? <span>{工作区查询.data.provider}</span> : null}
          </div>
        </div>

        {上下文Warning ? (
          <div className={`context-warning ${上下文Warning.level}`} role="status">
            {上下文Warning.text}
          </div>
        ) : null}

        {错误信息 ? (
          <div className="inline-alert" role="alert">
            {错误信息}
          </div>
        ) : null}

        <对话流 blocks={状态.blocks} currentSession={当前会话ID ? 工作区查询.data : undefined} />

        <发送框
          value={输入内容}
          disabled={提交消息动作.isPending}
          onChange={设置输入内容}
          onSubmit={处理发送消息}
        />
      </section>
    </div>
  );
}

function 会话列表栏(props: {
  sessions: 会话项[];
  selectedSessionId: string;
  workspaceRoot: string;
  loading: boolean;
  onWorkspaceRootChange: (value: string) => void;
  onCreate: () => void;
  onSelect: (sessionId: string) => void;
}) {
  return (
    <aside className="session-rail">
      <div className="rail-header">
        <div>
          <div className="panel-title">会话</div>
          <div className="panel-subtitle">左侧切换任务，右侧连续阅读执行过程。</div>
        </div>
        <button className="ghost-button" type="button" onClick={props.onCreate}>
          新建会话
        </button>
      </div>

      <label className="field-group">
        <span>工作区路径</span>
        <input
          value={props.workspaceRoot}
          onChange={(event) => props.onWorkspaceRootChange(event.target.value)}
          placeholder="E:/project/go-agent/.worktrees/minimal-runtime-loop"
        />
      </label>

      <div className="session-list">
        {props.loading ? <div className="empty-card">正在加载会话列表...</div> : null}
        {!props.loading && props.sessions.length === 0 ? (
          <div className="empty-card">还没有会话。先输入工作区路径，然后点击“新建会话”。</div>
        ) : null}
        {props.sessions.map((session) => (
          <button
            key={session.id}
            className={session.id === props.selectedSessionId ? "session-row active" : "session-row"}
            onClick={() => props.onSelect(session.id)}
            type="button"
          >
            <div className="session-row-top">
              <span className="session-title">{session.title || "未命名会话"}</span>
              <span className="session-time">{格式化时间(session.updated_at)}</span>
            </div>
            <div className="session-preview">{session.last_preview || "等待第一条消息..."}</div>
          </button>
        ))}
      </div>
    </aside>
  );
}

function 对话流(props: { blocks: 时间线块[]; currentSession?: { workspace_root: string; provider?: string; model?: string } }) {
  const mergedBlocks = mergeAdjacentAssistantBlocks(props.blocks);

  if (mergedBlocks.length === 0) {
    return (
      <div className="stream-empty">
        <h2>开始一个任务</h2>
        <p>这里会连续显示用户消息、tool call、最终输出以及运行时通知。</p>
        {props.currentSession?.workspace_root ? <div className="quiet-meta">{props.currentSession.workspace_root}</div> : null}
      </div>
    );
  }

  return (
    <div className="stream-list">
      {mergedBlocks.map((block) => (
        <对话块 key={block.id} block={block} />
      ))}
    </div>
  );
}

function 对话块(props: { block: 时间线块 }) {
  const { block } = props;
  if (block.kind === "tool_call") {
    return <ToolCallCard block={block} />;
  }

  const titleMap: Record<时间线块["kind"], string> = {
    user_message: "用户",
    reasoning: "思考",
    assistant_output: "最终输出",
    notice: "系统通知",
    error: "错误",
    tool_call: "工具调用",
  };

  return (
    <article className={`stream-block ${block.kind}`}>
      <div className="block-header">
        <span>{titleMap[block.kind]}</span>
        {block.status ? <span className="block-status">{block.status}</span> : null}
      </div>
      <div className="block-body">{block.text}</div>
      {block.usage ? (
        <div className="usage-row">
          <span>input {block.usage.input_tokens}</span>
          <span>output {block.usage.output_tokens}</span>
          <span>cached {block.usage.cached_tokens}</span>
        </div>
      ) : null}
    </article>
  );
}

export function ToolCallCard(props: { block: 时间线块 }) {
  const { block } = props;
  const isRunning = block.status === "running";
  const hasInlineResult =
    Boolean(block.result_preview) && 获取工具结果长度(block.result_preview) <= TOOL_RESULT_INLINE_THRESHOLD;
  const [isOpen, setIsOpen] = useState(() => isRunning || hasInlineResult);
  const summaryText = !hasInlineResult ? 生成工具结果摘要(block.result_preview) : "";

  useEffect(() => {
    setIsOpen(isRunning || hasInlineResult);
  }, [block.id, hasInlineResult, isRunning]);

  return (
    <details
      className="stream-block tool-card"
      open={isRunning || isOpen}
      onToggle={(event) => {
        if (isRunning) {
          return;
        }
        setIsOpen(event.currentTarget.open);
      }}
    >
      <summary className="tool-summary">
        <div className="tool-summary-main">
          <span>{block.tool_name || "tool call"}</span>
          {summaryText ? <span className="tool-result-preview">{summaryText}</span> : null}
        </div>
        <span className="block-status">{block.status || "idle"}</span>
      </summary>
      {isRunning || isOpen ? (
        <>
          {block.args_preview ? <pre className="tool-panel">{block.args_preview}</pre> : null}
          {block.result_preview ? (
            <div className={hasInlineResult ? "tool-result tool-result-inline" : "tool-result"}>
              {block.result_preview}
            </div>
          ) : null}
        </>
      ) : null}
    </details>
  );
}

export function Composer(props: {
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
  onSubmit: () => Promise<boolean | void> | boolean | void;
  inputId?: string;
  labelText?: string;
  sectionText?: string;
  submitAriaLabel?: string;
}) {
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [draftValue, setDraftValue] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (historyIndex === -1) {
      setDraftValue(props.value);
    }
  }, [historyIndex, props.value]);

  function 处理输入变化(value: string) {
    if (historyIndex !== -1) {
      setHistoryIndex(-1);
    }
    setDraftValue(value);
    props.onChange(value);
  }

  function 处理方向键(event: ReactKeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "ArrowUp") {
      if (输入历史记录.length === 0) {
        return;
      }
      event.preventDefault();
      if (historyIndex === -1) {
        const nextIndex = 输入历史记录.length - 1;
        setDraftValue(props.value);
        setHistoryIndex(nextIndex);
        props.onChange(输入历史记录[nextIndex]);
        return;
      }
      const nextIndex = Math.max(0, historyIndex - 1);
      setHistoryIndex(nextIndex);
      props.onChange(输入历史记录[nextIndex]);
      return;
    }

    if (event.key === "ArrowDown" && historyIndex !== -1) {
      event.preventDefault();
      if (historyIndex >= 输入历史记录.length - 1) {
        setHistoryIndex(-1);
        props.onChange(draftValue);
        return;
      }
      const nextIndex = historyIndex + 1;
      setHistoryIndex(nextIndex);
      props.onChange(输入历史记录[nextIndex]);
    }
  }

  async function 处理提交() {
    const message = props.value.trim();
    if (!message) {
      return;
    }

    const draftBeforeSubmit = props.value;
    setIsSubmitting(true);
    props.onChange("");
    setHistoryIndex(-1);
    setDraftValue("");

    try {
      const result = await props.onSubmit();
      if (result === false) {
        props.onChange(draftBeforeSubmit);
        setDraftValue(draftBeforeSubmit);
        return;
      }
      记录输入历史(message);
    } catch (error) {
      props.onChange(draftBeforeSubmit);
      setDraftValue(draftBeforeSubmit);
      throw error;
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="composer">
      {props.sectionText ? <div className="composer-section-tag">{props.sectionText}</div> : null}
      <label className="composer-label" htmlFor={props.inputId ?? "message-box"}>
        {props.labelText ?? "输入指令"}
      </label>
      <textarea
        id={props.inputId ?? "message-box"}
        className="composer-input"
        value={props.value}
        disabled={props.disabled || isSubmitting}
        onChange={(event) => 处理输入变化(event.target.value)}
        onKeyDown={处理方向键}
        placeholder="例如：检查当前工作区里最近的 provider 改动，并总结风险。"
        rows={4}
      />
      <div className="composer-actions">
        <span className="composer-hint">按你的真实工作方式提问，不需要先切到统计页。</span>
        <button
          aria-label={props.submitAriaLabel}
          className="primary-button"
          disabled={props.disabled || isSubmitting}
          onClick={() => {
            void 处理提交();
          }}
          type="button"
        >
          发送
        </button>
      </div>
    </div>
  );
}

function 发送框(props: {
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
  onSubmit: () => Promise<boolean | void> | boolean | void;
}) {
  return (
    <Composer
      {...props}
      inputId="chat-message-box"
      labelText="任务输入"
      sectionText="输入指令"
      submitAriaLabel="发送消息"
    />
  );
}

function 统计页面() {
  const [searchParams, setSearchParams] = useSearchParams();
  const 当前会话ID = searchParams.get("session") ?? "";

  const 会话查询 = useQuery({
    queryKey: ["sessions"],
    queryFn: 获取会话列表,
  });
  const 概览查询 = useQuery({
    queryKey: ["metrics-overview", 当前会话ID],
    queryFn: () => 获取统计概览(当前会话ID || undefined),
  });
  const 时序查询 = useQuery({
    queryKey: ["metrics-timeseries", 当前会话ID],
    queryFn: () => 获取统计时序(当前会话ID || undefined),
  });
  const 明细查询 = useQuery({
    queryKey: ["metrics-turns", 当前会话ID],
    queryFn: () => 获取统计明细(当前会话ID || undefined, 1, 20),
  });

  const overview = 概览查询.data ?? {
    input_tokens: 0,
    output_tokens: 0,
    cached_tokens: 0,
    cache_hit_rate: 0,
  };

  return (
    <section className="metrics-layout">
      <div className="metrics-header">
        <div>
          <h1 className="section-title">Token 统计</h1>
          <p className="section-subtitle">把 input、output 和 cached 的变化单独拿出来看，避免干扰主对话页。</p>
        </div>
        <label className="field-group compact">
          <span>会话筛选</span>
          <select
            value={当前会话ID}
            onChange={(event) => {
              startTransition(() => {
                setSearchParams((current) => {
                  const next = new URLSearchParams(current);
                  if (event.target.value) {
                    next.set("session", event.target.value);
                  } else {
                    next.delete("session");
                  }
                  return next;
                });
              });
            }}
          >
            <option value="">全部会话</option>
            {(会话查询.data?.sessions ?? []).map((session) => (
              <option key={session.id} value={session.id}>
                {session.title || session.id}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="metrics-cards">
        <统计卡片 title="input tokens" value={overview.input_tokens} />
        <统计卡片 title="output tokens" value={overview.output_tokens} />
        <统计卡片 title="cached tokens" value={overview.cached_tokens} />
        <统计卡片 title="cache hit rate" value={`${(overview.cache_hit_rate * 100).toFixed(1)}%`} />
      </div>

      <section className="chart-card">
        <div className="panel-title">Token 趋势</div>
        <div className="chart-wrap">
          {时序查询.data?.points.length ? (
            <ResponsiveContainer width="100%" height={280}>
              <LineChart data={时序查询.data.points}>
                <CartesianGrid stroke="rgba(148, 163, 184, 0.12)" vertical={false} />
                <XAxis
                  dataKey="bucket_start"
                  tickFormatter={(value) => String(value).slice(5, 10)}
                  stroke="#7b8ca5"
                />
                <YAxis stroke="#7b8ca5" />
                <Tooltip
                  contentStyle={{
                    background: "#121922",
                    border: "1px solid rgba(148, 163, 184, 0.2)",
                    borderRadius: 16,
                  }}
                />
                <Line dataKey="input_tokens" name="input" stroke="#f59e0b" strokeWidth={2.2} dot={false} />
                <Line dataKey="output_tokens" name="output" stroke="#38bdf8" strokeWidth={2.2} dot={false} />
                <Line dataKey="cached_tokens" name="cached" stroke="#34d399" strokeWidth={2.2} dot={false} />
              </LineChart>
            </ResponsiveContainer>
          ) : (
            <div className="empty-card">暂无 token 趋势数据。</div>
          )}
        </div>
      </section>

      <section className="table-card">
        <div className="panel-title">按 turn 查看</div>
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>会话</th>
                <th>turn</th>
                <th>provider</th>
                <th>model</th>
                <th>input</th>
                <th>output</th>
                <th>cached</th>
                <th>hit rate</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {(明细查询.data?.items ?? []).map((row) => (
                <tr key={row.turn_id}>
                  <td>{row.session_title || row.session_id}</td>
                  <td>{row.turn_id}</td>
                  <td>{row.provider}</td>
                  <td>{row.model}</td>
                  <td>{row.input_tokens}</td>
                  <td>{row.output_tokens}</td>
                  <td>{row.cached_tokens}</td>
                  <td>{(row.cache_hit_rate * 100).toFixed(1)}%</td>
                  <td>{格式化时间(row.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {!明细查询.data?.items.length ? <div className="empty-card">还没有可展示的 turn usage。</div> : null}
        </div>
      </section>
    </section>
  );
}

function 统计卡片(props: { title: string; value: number | string }) {
  return (
    <article className="metric-card">
      <div className="metric-title">{props.title}</div>
      <div className="metric-value">{props.value}</div>
    </article>
  );
}

function 连接文案(status: "idle" | "connecting" | "open" | "reconnecting") {
  switch (status) {
    case "open":
      return "实时已连接";
    case "reconnecting":
      return "正在重连";
    case "connecting":
      return "正在连接";
    default:
      return "等待连接";
  }
}

function 格式化时间(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
