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
    },
    composer: {
      placeholder: "Send a message...",
      send: "Send",
      sending: "...",
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
        pendingReports: "Pending Reports",
        pendingReportsDetail: "{count} mailbox items",
        approvals: "Approval Requests",
        approvalsDetail: "Runtime permission waits",
      },
      panels: {
        diagnosticsTitle: "Diagnostics",
        diagnosticsSubtitle: "Runtime graph diagnostics surfaced as first-class runtime events",
        diagnosticsEmpty: "No diagnostics were emitted for this workspace.",
        contextTitle: "Context Heads",
        contextSubtitle: "Current session history branches",
        contextEmpty: "No context heads recorded yet.",
        reportsTitle: "Pending Reports",
        reportsSubtitle: "Workspace mailbox deliveries waiting on the parent flow",
        reportsEmpty: "No reports waiting.",
        tasksTitle: "Tasks",
        tasksSubtitle: "Workspace execution spine",
        tasksEmpty: "No tasks recorded yet.",
        approvalsTitle: "Approval Queue",
        approvalsSubtitle: "Permission requests currently blocking execution",
        approvalsEmpty: "No approval requests are waiting.",
        incidentsTitle: "Incidents",
        incidentsSubtitle: "Automation incidents currently attached to this workspace",
        incidentsEmpty: "No incidents recorded yet.",
        dispatchTitle: "Dispatch Attempts",
        dispatchSubtitle: "Child-agent dispatches and approval-gated execution handoffs",
        dispatchEmpty: "No dispatch attempts recorded yet.",
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
    },
    composer: {
      placeholder: "发送消息...",
      send: "发送",
      sending: "...",
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
        pendingReports: "待处理报告",
        pendingReportsDetail: "共 {count} 个邮箱条目",
        approvals: "审批请求",
        approvalsDetail: "运行时权限等待项",
      },
      panels: {
        diagnosticsTitle: "诊断",
        diagnosticsSubtitle: "作为一等运行时事件展示的 runtime graph 诊断信息",
        diagnosticsEmpty: "当前工作区没有诊断事件。",
        contextTitle: "上下文分支",
        contextSubtitle: "当前会话的历史分支",
        contextEmpty: "还没有记录任何上下文分支。",
        reportsTitle: "待处理报告",
        reportsSubtitle: "等待父流程消费的工作区邮箱投递项",
        reportsEmpty: "没有待处理报告。",
        tasksTitle: "任务",
        tasksSubtitle: "工作区执行主线",
        tasksEmpty: "还没有记录任何任务。",
        approvalsTitle: "审批队列",
        approvalsSubtitle: "当前阻塞执行的权限请求",
        approvalsEmpty: "没有待处理的审批请求。",
        incidentsTitle: "事件",
        incidentsSubtitle: "当前附着在工作区上的自动化事件",
        incidentsEmpty: "还没有记录任何事件。",
        dispatchTitle: "派发尝试",
        dispatchSubtitle: "子代理派发与审批门控的执行交接",
        dispatchEmpty: "还没有记录任何派发尝试。",
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
