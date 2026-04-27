import { createContext, useContext, useEffect, useState } from "react";

export type Locale = "en-US" | "zh-CN";

interface MessageTree {
  [key: string]: string | MessageTree;
}

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}

const STORAGE_KEY = "sesame-console.locale";

const messages: Record<Locale, MessageTree> = {
  "en-US": {
    nav: {
      chat: "Chat",
      runtime: "Runtime",
      usage: "Usage",
      roles: "Roles",
    },
    app: {
      currentWorkspace: "Current workspace",
      language: "Language",
    },
    sidebar: {
      title: "Workspace",
      currentBinding: "Current binding",
      waitingMetadata: "Waiting for workspace metadata...",
      bindingDescription:
        "Chat, usage, and roles all operate on the current workspace runtime. Context-head navigation will live here instead of legacy session selection.",
      session: "Session",
      status: "Status",
      connected: "Connected",
      connecting: "Connecting",
      reconnecting: "Reconnecting",
      error: "Error",
      idle: "Idle",
    },
    composer: {
      placeholder: "Send a message...",
      send: "Send",
      sending: "...",
    },
    chat: {
      emptyPrompt: "Send a message to start the conversation.",
      newMessages: "New messages",
      connecting: "Connecting...",
      reconnecting: "Reconnecting...",
      connected: "Connected",
      error: "Error",
      idle: "Idle",
      suggestions: {
        explainCodebase: "Explain this codebase",
        runTests: "Run the tests",
        checkGitStatus: "Check git status",
      },
    },
    usage: {
      title: "Token Usage",
      currentSession: "Current session",
      allSessions: "All sessions",
      last30Days: "last 30 days",
    },
    roles: {
      title: "Roles",
      newRole: "New role",
      loading: "Loading...",
      empty: "No roles yet.",
      saveFailed: "Failed to save role.",
      deleteFailed: "Failed to delete role.",
      loadingRole: "Loading role...",
      loadFailed: "Failed to load role details for {roleID}.",
      retry: "Retry",
    },
    runtime: {
      loading: "Loading workspace runtime...",
      title: "Workspace Runtime",
      subtitle: "Context history, task execution, and report delivery for the current workspace.",
      summary: {
        contextHeads: "Context Heads",
        contextHeadsDetail: "Available history branches",
        activeTasks: "Active Tasks",
        activeTasksDetail: "{count} total tracked tasks",
        diagnostics: "Diagnostics",
        diagnosticsDetail: "Runtime graph events that need attention",
        queuedReports: "Queued Reports",
        queuedReportsDetail: "{count} report items",
      },
      panels: {
        diagnosticsTitle: "Diagnostics",
        diagnosticsSubtitle: "Runtime graph diagnostics surfaced as first-class runtime events",
        diagnosticsEmpty: "No diagnostics were emitted for this workspace.",
        contextTitle: "Context Heads",
        contextSubtitle: "Current session history branches",
        contextEmpty: "No context heads recorded yet.",
        reportsTitle: "Queued Reports",
        reportsSubtitle: "Workspace report deliveries waiting on the agent flow",
        reportsEmpty: "No reports waiting.",
        tasksTitle: "Tasks",
        tasksSubtitle: "Workspace execution spine",
        tasksEmpty: "No tasks recorded yet.",
        toolRunsTitle: "Tool Runs",
        toolRunsSubtitle: "Tool execution detail attached to active tasks",
        toolRunsEmpty: "No tool runs recorded yet.",
        worktreesTitle: "Worktrees",
        worktreesSubtitle: "Attached worktrees created for task execution",
        worktreesEmpty: "No worktrees recorded yet.",
        detailTitle: "Selection Detail",
        detailSubtitle: "Inspect one runtime asset at a time",
        detailEmpty: "Choose a runtime asset to inspect its details.",
      },
    },
  },
  "zh-CN": {
    nav: {
      chat: "对话",
      runtime: "运行时",
      usage: "用量",
      roles: "角色",
    },
    app: {
      currentWorkspace: "当前工作区",
      language: "语言",
    },
    sidebar: {
      title: "工作区",
      currentBinding: "当前绑定",
      waitingMetadata: "等待工作区元数据...",
      bindingDescription:
        "对话、用量和角色都作用于当前工作区运行时。上下文分支导航会放在这里，而不是旧的 session 切换。",
      session: "会话",
      status: "状态",
      connected: "已连接",
      connecting: "连接中",
      reconnecting: "重连中",
      error: "错误",
      idle: "空闲",
    },
    composer: {
      placeholder: "发送消息...",
      send: "发送",
      sending: "...",
    },
    chat: {
      emptyPrompt: "发送消息开始对话。",
      newMessages: "新消息",
      connecting: "连接中...",
      reconnecting: "重连中...",
      connected: "已连接",
      error: "错误",
      idle: "空闲",
      suggestions: {
        explainCodebase: "解释这个代码库",
        runTests: "运行测试",
        checkGitStatus: "检查 git 状态",
      },
    },
    usage: {
      title: "Token 用量",
      currentSession: "当前会话",
      allSessions: "所有会话",
      last30Days: "最近 30 天",
    },
    roles: {
      title: "角色",
      newRole: "新建角色",
      loading: "加载中...",
      empty: "还没有角色。",
      saveFailed: "保存角色失败。",
      deleteFailed: "删除角色失败。",
      loadingRole: "正在加载角色...",
      loadFailed: "加载角色 {roleID} 的详情失败。",
      retry: "重试",
    },
    runtime: {
      loading: "正在加载工作区运行时...",
      title: "工作区运行时",
      subtitle: "查看当前工作区的上下文历史、任务执行和报告投递。",
      summary: {
        contextHeads: "上下文分支",
        contextHeadsDetail: "可用的历史分支",
        activeTasks: "活跃任务",
        activeTasksDetail: "共跟踪 {count} 个任务",
        diagnostics: "诊断",
        diagnosticsDetail: "需要关注的运行时图事件",
        queuedReports: "待处理报告",
        queuedReportsDetail: "共 {count} 条报告",
      },
      panels: {
        diagnosticsTitle: "诊断",
        diagnosticsSubtitle: "作为一等运行时事件展示的 runtime graph 诊断信息",
        diagnosticsEmpty: "当前工作区没有诊断事件。",
        contextTitle: "上下文分支",
        contextSubtitle: "当前会话的历史分支",
        contextEmpty: "还没有记录任何上下文分支。",
        reportsTitle: "待处理报告",
        reportsSubtitle: "等待 agent 流程消费的工作区报告投递",
        reportsEmpty: "没有待处理报告。",
        tasksTitle: "任务",
        tasksSubtitle: "工作区执行主线",
        tasksEmpty: "还没有记录任何任务。",
        toolRunsTitle: "工具执行",
        toolRunsSubtitle: "附着在活跃任务上的工具执行详情",
        toolRunsEmpty: "还没有记录任何工具执行。",
        worktreesTitle: "工作树",
        worktreesSubtitle: "为任务执行创建的附属工作树",
        worktreesEmpty: "还没有记录任何工作树。",
        detailTitle: "详情",
        detailSubtitle: "一次只检查一个运行时对象",
        detailEmpty: "请选择一个运行时对象查看详情。",
      },
    },
  },
};

const defaultContext: I18nContextValue = {
  locale: "en-US",
  setLocale: () => {},
  t: (key, vars) => formatMessage(resolveMessage(messages["en-US"], key) ?? key, vars),
};

const I18nContext = createContext<I18nContextValue>(defaultContext);

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocale] = useState<Locale>(() => detectInitialLocale());

  useEffect(() => {
    document.documentElement.lang = locale;
    window.localStorage.setItem(STORAGE_KEY, locale);
  }, [locale]);

  function t(key: string, vars?: Record<string, string | number>) {
    const bundle = messages[locale] ?? messages["en-US"];
    return formatMessage(resolveMessage(bundle, key) ?? key, vars);
  }

  return (
    <I18nContext.Provider value={{ locale, setLocale, t }}>{children}</I18nContext.Provider>
  );
}

export function useI18n() {
  return useContext(I18nContext);
}

function detectInitialLocale(): Locale {
  if (typeof window !== "undefined") {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === "en-US" || stored === "zh-CN") {
      return stored;
    }
    if (window.navigator.language.toLowerCase().startsWith("zh")) {
      return "zh-CN";
    }
  }
  return "en-US";
}

function resolveMessage(bundle: MessageTree, key: string): string | null {
  const path = key.split(".");
  let current: string | MessageTree | undefined = bundle;
  for (const segment of path) {
    if (typeof current !== "object" || current === null || !(segment in current)) {
      return null;
    }
    current = current[segment];
  }
  return typeof current === "string" ? current : null;
}

function formatMessage(
  template: string,
  vars?: Record<string, string | number>,
): string {
  if (!vars) {
    return template;
  }
  return template.replace(/\{(\w+)\}/g, (_, key: string) => String(vars[key] ?? ""));
}
