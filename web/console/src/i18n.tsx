import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";

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
      roles: "Roles",
      tasks: "Tasks",
      context: "Context",
      automations: "Automations",
      workflows: "Workflows",
      reports: "Reports",
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
        "Chat, roles, automations, and reports operate on the current workspace runtime.",
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
    roles: {
      title: "Roles",
      loading: "Loading...",
      empty: "No roles yet.",
      loadingRole: "Loading role...",
      loadFailed: "Failed to load role details for {roleID}.",
      retry: "Retry",
      create: "Create role",
      createRole: "Create Role",
      editRole: "Edit Role",
      edit: "Edit",
      save: "Save",
      saving: "Saving...",
      cancel: "Cancel",
      testRun: "Test Run",
      runTest: "Run Test",
      starting: "Starting...",
      defaultTestPrompt: "Run a short self-check for role {roleID}. Reply with your role identity and whether your configured tools and permissions are available.",
      testPromptPlaceholder: "Optional test prompt for {roleID}. Leave empty to run the default self-check.",
      recentRuns: "Recent Runs",
      recentRunsFailed: "Failed to load recent runs.",
      noRecentRuns: "No runs for this role yet.",
    },
    automations: {
      title: "Automations",
      subtitle: "Scheduled workspace automation rules.",
      empty: "No automations configured.",
      noWorkspace: "Waiting for workspace session.",
      loadFailed: "Failed to load automations.",
      create: "Create automation",
      cancelCreate: "Cancel",
      save: "Save automation",
      saving: "Saving...",
      pause: "Pause",
      resume: "Resume",
      updated: "Updated",
      runs: "Recent runs",
      noRuns: "No runs recorded.",
      runsFailed: "Failed to load automation runs.",
      form: {
        title: "Title",
        goal: "Goal",
        owner: "Owner role",
        selectRole: "Select a role",
        watcherPath: "Watcher path",
        watcherCron: "Watcher cron",
      },
    },
    workflows: {
      title: "Workflows",
      subtitle: "Manual workflow templates and auditable runs.",
      create: "Create",
      createTitle: "New Workflow",
      detailSubtitle: "Edit the workflow definition, then trigger manual runs from the console.",
      draft: "Draft",
      untitled: "Untitled workflow",
      save: "Save workflow",
      saving: "Saving...",
      trigger: "Trigger",
      triggering: "Triggering...",
      retry: "Retry",
      noWorkspace: "Waiting for workspace session.",
      empty: "No workflows configured.",
      loadFailed: "Failed to load workflows.",
      recentRuns: "Recent Runs",
      recentRunsSubtitle: "Latest 20 runs for the selected workflow.",
      refreshRuns: "Refresh",
      saveToViewRuns: "Save the workflow to view run history.",
      runsLoadFailed: "Failed to load workflow runs.",
      noRuns: "No runs recorded.",
      neverRun: "Never run",
      updated: "Updated",
      ownerRole: "Owner",
      stepCount: "{count} step(s)",
      validation: {
        required: "Name and steps are required.",
      },
      form: {
        name: "Name",
        trigger: "Trigger",
        ownerRole: "Owner role",
        selectRole: "Select a role",
        prompt: "Template prompt",
        steps: "Steps JSON",
        generateTemplate: "Use template",
        defaultPrompt: "Review the current workspace change and return a concise report.",
        triggerOptions: {
          manual: "manual",
          schedule: "schedule",
          watcher: "watcher",
          webhook: "webhook",
          file_change: "file_change",
        },
      },
      run: {
        taskIds: "Task IDs",
        reportIds: "Report IDs",
        trace: "Trace",
        event: "Event",
        state: "State",
        kind: "Kind",
        taskId: "Task ID",
        approvalId: "Approval ID",
        message: "Message",
        time: "Time",
      },
    },
    reports: {
      title: "Reports",
      subtitle: "Workspace reports emitted by agent tasks.",
      empty: "No reports yet.",
      noWorkspace: "Waiting for workspace session.",
      loadFailed: "Failed to load reports.",
      queued: "{count} queued",
    },
    context: {
      title: "Context",
      subtitle: "Project state and durable workspace memory.",
      noWorkspace: "Waiting for workspace session.",
      noSession: "Waiting for an active session.",
      inspector: "Context Inspector",
      inspectorSubtitle: "Read-only preview of prompt input and available context blocks.",
      generated: "Generated {time}",
      previewLoadFailed: "Failed to load context preview.",
      promptTokens: "Prompt tokens",
      includedBlocks: "Included blocks",
      availableBlocks: "Available blocks",
      promptPreview: "Prompt preview",
      contextBlocks: "Context blocks",
      noPromptPreview: "No prompt preview available.",
      noContextBlocks: "No context blocks available.",
      tokens: "{count} tokens",
      projectState: "Project State",
      updated: "Updated {time}",
      notSaved: "Not saved yet",
      autoUpdate: "Auto-update project state",
      saving: "Saving...",
      saveProjectState: "Save Project State",
      memory: "Memory",
      memorySubtitle: "Durable notes used for recall across long-running work.",
      refresh: "Refresh",
      searchPlaceholder: "Search memory, or leave empty to list recent items.",
      memoryLoadFailed: "Failed to load memory.",
      noMemories: "No memory entries yet.",
      addMemory: "Add Memory",
      kind: "Kind",
      content: "Content",
      source: "Source",
      confidence: "Confidence {value}",
      saveMemory: "Save Memory",
      delete: "Delete",
    },
    tasks: {
      title: "Tasks",
      subtitle: "Workspace task monitor for role runs and background work.",
      traceTitle: "Task Trace",
      backToTasks: "Tasks",
      refresh: "Refresh",
      cancel: "Cancel",
      active: "Active",
      completed: "Completed",
      failed: "Failed",
      total: "Total",
      updated: "Updated",
      noWorkspace: "Waiting for workspace session.",
      listLoadFailed: "Failed to load tasks.",
      emptyList: "No tasks yet.",
      loadFailed: "Failed to load task trace.",
      emptyTrace: "No task trace available.",
      taskState: "Task",
      turnState: "Turn",
      sessionState: "Session",
      role: "Role",
      linkage: "Runtime links",
      parentSession: "Parent session",
      parentTurn: "Parent turn",
      roleSession: "Role session",
      roleTurn: "Role turn",
      reportSession: "Report session",
      outputPath: "Output path",
      prompt: "Prompt",
      finalText: "Final text",
      events: "Events",
      messages: "Messages",
      reports: "Reports",
      logPreview: "Log preview",
      noEvents: "No events recorded.",
      noMessages: "No messages recorded.",
      noReports: "No reports recorded.",
      noLog: "No log output yet.",
      truncated: "truncated",
      filters: {
        all: "All",
        active: "Active",
        failed: "Failed",
        completed: "Completed",
      },
    },
  },
  "zh-CN": {
    nav: {
      chat: "对话",
      roles: "角色",
      tasks: "任务",
      context: "上下文",
      automations: "自动化",
      workflows: "工作流",
      reports: "报告",
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
        "对话、角色、自动化和报告都作用于当前工作区运行时。",
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
    roles: {
      title: "角色",
      loading: "加载中...",
      empty: "还没有角色。",
      loadingRole: "正在加载角色...",
      loadFailed: "加载角色 {roleID} 的详情失败。",
      retry: "重试",
      create: "创建角色",
      createRole: "创建角色",
      editRole: "编辑角色",
      edit: "编辑",
      save: "保存",
      saving: "保存中...",
      cancel: "取消",
      testRun: "测试运行",
      runTest: "运行测试",
      starting: "启动中...",
      defaultTestPrompt: "请对角色 {roleID} 执行一次简短自检。回复你的角色身份，以及当前工具和权限配置是否可用。",
      testPromptPlaceholder: "可选的 {roleID} 测试提示词。留空则运行默认自检。",
      recentRuns: "最近运行",
      recentRunsFailed: "加载最近运行失败。",
      noRecentRuns: "这个角色还没有运行记录。",
    },
    automations: {
      title: "自动化",
      subtitle: "当前工作区的计划自动化规则。",
      empty: "还没有配置自动化。",
      noWorkspace: "正在等待工作区会话。",
      loadFailed: "加载自动化失败。",
      create: "创建自动化",
      cancelCreate: "取消",
      save: "保存自动化",
      saving: "保存中...",
      pause: "暂停",
      resume: "恢复",
      updated: "更新时间",
      runs: "最近运行",
      noRuns: "还没有运行记录。",
      runsFailed: "加载自动化运行记录失败。",
      form: {
        title: "标题",
        goal: "目标",
        owner: "所属角色",
        selectRole: "选择角色",
        watcherPath: "Watcher 路径",
        watcherCron: "Watcher Cron",
      },
    },
    workflows: {
      title: "工作流",
      subtitle: "手动工作流模板和可审计运行记录。",
      create: "创建",
      createTitle: "新建工作流",
      detailSubtitle: "在控制台编辑工作流定义，并手动触发运行。",
      draft: "草稿",
      untitled: "未命名工作流",
      save: "保存工作流",
      saving: "保存中...",
      trigger: "触发",
      triggering: "触发中...",
      retry: "重试",
      noWorkspace: "正在等待工作区会话。",
      empty: "还没有配置工作流。",
      loadFailed: "加载工作流失败。",
      recentRuns: "最近运行",
      recentRunsSubtitle: "当前选中工作流最近 20 次运行。",
      refreshRuns: "刷新",
      saveToViewRuns: "先保存工作流，再查看运行历史。",
      runsLoadFailed: "加载工作流运行记录失败。",
      noRuns: "还没有运行记录。",
      neverRun: "未运行",
      updated: "更新时间",
      ownerRole: "所属角色",
      stepCount: "{count} 个步骤",
      validation: {
        required: "名称和 steps 不能为空。",
      },
      form: {
        name: "名称",
        trigger: "触发方式",
        ownerRole: "所属角色",
        selectRole: "选择角色",
        prompt: "模板提示词",
        steps: "Steps JSON",
        generateTemplate: "生成模板",
        defaultPrompt: "检查当前工作区改动，并返回简洁结论。",
        triggerOptions: {
          manual: "manual",
          schedule: "schedule",
          watcher: "watcher",
          webhook: "webhook",
          file_change: "file_change",
        },
      },
      run: {
        taskIds: "任务 ID",
        reportIds: "报告 ID",
        trace: "运行追踪",
        event: "事件",
        state: "状态",
        kind: "类型",
        taskId: "任务 ID",
        approvalId: "审批 ID",
        message: "消息",
        time: "时间",
      },
    },
    reports: {
      title: "报告",
      subtitle: "Agent 任务生成的工作区报告。",
      empty: "还没有报告。",
      noWorkspace: "正在等待工作区会话。",
      loadFailed: "加载报告失败。",
      queued: "{count} 条待处理",
    },
    context: {
      title: "上下文",
      subtitle: "项目状态和持久化工作区记忆。",
      noWorkspace: "正在等待工作区会话。",
      noSession: "正在等待活跃会话。",
      inspector: "上下文检查器",
      inspectorSubtitle: "只读预览模型输入和可用上下文块。",
      generated: "生成于 {time}",
      previewLoadFailed: "加载上下文预览失败。",
      promptTokens: "Prompt token",
      includedBlocks: "已注入块",
      availableBlocks: "可用块",
      promptPreview: "Prompt 预览",
      contextBlocks: "上下文块",
      noPromptPreview: "没有可用的 prompt 预览。",
      noContextBlocks: "没有可用的上下文块。",
      tokens: "{count} tokens",
      projectState: "项目状态",
      updated: "更新于 {time}",
      notSaved: "尚未保存",
      autoUpdate: "自动更新项目状态",
      saving: "保存中...",
      saveProjectState: "保存项目状态",
      memory: "记忆",
      memorySubtitle: "用于长期工作召回的持久化笔记。",
      refresh: "刷新",
      searchPlaceholder: "搜索记忆，留空则列出最近项目。",
      memoryLoadFailed: "加载记忆失败。",
      noMemories: "还没有记忆。",
      addMemory: "添加记忆",
      kind: "类型",
      content: "内容",
      source: "来源",
      confidence: "置信度 {value}",
      saveMemory: "保存记忆",
      delete: "删除",
    },
    tasks: {
      title: "任务",
      subtitle: "当前工作区的角色运行和后台任务监控。",
      traceTitle: "任务追踪",
      backToTasks: "任务",
      refresh: "刷新",
      cancel: "取消",
      active: "运行中",
      completed: "已完成",
      failed: "失败",
      total: "总数",
      updated: "更新时间",
      noWorkspace: "正在等待工作区会话。",
      listLoadFailed: "加载任务失败。",
      emptyList: "还没有任务。",
      loadFailed: "加载任务追踪失败。",
      emptyTrace: "没有可用的任务追踪。",
      taskState: "任务",
      turnState: "轮次",
      sessionState: "会话",
      role: "角色",
      linkage: "运行关联",
      parentSession: "主会话",
      parentTurn: "主轮次",
      roleSession: "角色会话",
      roleTurn: "角色轮次",
      reportSession: "报告会话",
      outputPath: "输出路径",
      prompt: "提示词",
      finalText: "最终文本",
      events: "事件",
      messages: "消息",
      reports: "报告",
      logPreview: "日志预览",
      noEvents: "还没有事件记录。",
      noMessages: "还没有消息记录。",
      noReports: "还没有报告记录。",
      noLog: "还没有日志输出。",
      truncated: "已截断",
      filters: {
        all: "全部",
        active: "运行中",
        failed: "失败",
        completed: "已完成",
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

export function I18nProvider({ children }: { children: ReactNode }) {
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
