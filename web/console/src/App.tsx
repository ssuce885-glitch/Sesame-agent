import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import {
  createBrowserRouter,
  RouterProvider,
  useNavigate,
  useLocation,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Sidebar } from "./components/Sidebar";
import { ChatPage } from "./pages/ChatPage";
import { RolesPage } from "./pages/RolesPage";
import { AutomationsPage } from "./pages/AutomationsPage";
import { ReportsPage } from "./pages/ReportsPage";
import { TaskTracePage } from "./pages/TaskTracePage";
import { TasksPage } from "./pages/TasksPage";
import { ContextPage } from "./pages/ContextPage";
import { useSession } from "./api/queries";
import { useI18n, I18nProvider, type Locale } from "./i18n";
import { Globe } from "./components/Icon";

const DEFAULT_WORKSPACE_ROOT =
  (import.meta.env.VITE_WORKSPACE_ROOT as string | undefined) ?? "";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      staleTime: 10_000,
    },
  },
});

function RootLayout({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{children}</I18nProvider>
    </QueryClientProvider>
  );
}

export function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const { locale, setLocale } = useI18n();
  const { data: currentSession } = useSession(DEFAULT_WORKSPACE_ROOT);
  const activeSessionId = currentSession?.id ?? null;
  const workspaceRoot = currentSession?.workspace_root ?? DEFAULT_WORKSPACE_ROOT;
  const workspaceName = workspaceRoot ? workspaceRoot.split("/").filter(Boolean).pop() : "Sesame";
  const [sidebarConnection, setSidebarConnection] = useState<
    "idle" | "connecting" | "open" | "reconnecting" | "error"
  >("idle");

  const handleConnectionChange = useCallback(
    (value: "idle" | "connecting" | "open" | "reconnecting" | "error") => {
      setSidebarConnection(value);
    },
    [],
  );

  useEffect(() => {
    if (activeSessionId && location.pathname === "/") {
      navigate("/chat");
    }
  }, [activeSessionId, navigate, location.pathname]);

  useEffect(() => {
    if (location.pathname !== "/chat") {
      setSidebarConnection("idle");
    }
  }, [location.pathname]);

  const activePath =
    location.pathname === "/" ? "/chat" : location.pathname;
  const taskTraceId = activePath.startsWith("/tasks/")
    ? decodeURIComponent(activePath.slice("/tasks/".length))
    : "";
  const sidebarActivePath = activePath.startsWith("/tasks/") ? "/tasks" : activePath;

  return (
    <div className="flex h-screen" style={{ backgroundColor: "var(--color-bg)" }}>
      <Sidebar
        workspaceName={workspaceName}
        workspaceRoot={workspaceRoot}
        sessionId={activeSessionId ?? undefined}
        connection={sidebarConnection}
        activePath={sidebarActivePath}
        onNavigate={(path) => navigate(path)}
      />

      <div className="flex flex-col flex-1 overflow-hidden">
        {/* Top bar */}
        <header
          className="flex items-center justify-between h-11 px-4 shrink-0"
          style={{
            backgroundColor: "var(--color-bg-elevated)",
            borderBottom: "1px solid var(--color-border)",
          }}
        >
          <div className="flex items-center gap-2">
            <span
              className="text-xs"
              style={{ color: "var(--color-text-tertiary)" }}
            >
              {workspaceName || "Sesame"}
            </span>
            {workspaceRoot && (
              <>
                <span style={{ color: "var(--color-border-strong)" }}>/</span>
                <span
                  className="text-xs truncate max-w-[200px]"
                  style={{ color: "var(--color-text-tertiary)" }}
                >
                  {workspaceRoot}
                </span>
              </>
            )}
          </div>

          <div className="flex items-center gap-3">
            <LanguageSwitcher locale={locale} onChange={setLocale} />
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-hidden">
          {activePath === "/chat" && (
            <ChatPage
              sessionId={activeSessionId ?? ""}
              onConnectionChange={handleConnectionChange}
            />
          )}
          {activePath === "/roles" && <RolesPage workspaceRoot={workspaceRoot || null} />}
          {activePath === "/automations" && (
            <AutomationsPage workspaceRoot={workspaceRoot || null} />
          )}
          {activePath === "/reports" && (
            <ReportsPage workspaceRoot={workspaceRoot || null} />
          )}
          {activePath === "/tasks" && (
            <TasksPage workspaceRoot={workspaceRoot || null} />
          )}
          {activePath === "/context" && (
            <ContextPage workspaceRoot={workspaceRoot || null} sessionId={activeSessionId} />
          )}
          {taskTraceId && <TaskTracePage taskId={taskTraceId} />}
        </main>
      </div>
    </div>
  );
}

function LanguageSwitcher({
  locale,
  onChange,
}: {
  locale: Locale;
  onChange: (locale: Locale) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onChange(locale === "en-US" ? "zh-CN" : "en-US")}
      className="flex items-center gap-1.5 text-xs font-medium rounded px-2 py-1"
      style={{
        backgroundColor: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        color: "var(--color-text-secondary)",
        cursor: "pointer",
        transition: "border-color 0.15s",
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = "var(--color-border-strong)";
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = "var(--color-border)";
      }}
    >
      <Globe size={12} />
      {locale === "en-US" ? "EN" : "中文"}
    </button>
  );
}

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { path: "chat", element: null },
      { path: "roles", element: null },
      { path: "automations", element: null },
      { path: "reports", element: null },
      { path: "tasks", element: null },
      { path: "context", element: null },
      { path: "tasks/:taskId", element: null },
    ],
  },
]);

export default function App() {
  return (
    <RootLayout>
      <RouterProvider router={router} />
    </RootLayout>
  );
}
