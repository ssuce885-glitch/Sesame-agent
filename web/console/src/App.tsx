import { useCallback, useEffect, useState } from "react";
import {
  createBrowserRouter,
  RouterProvider,
  useNavigate,
  useLocation,
  Link,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Sidebar } from "./components/Sidebar";
import { ChatPage } from "./pages/ChatPage";
import { RuntimePage } from "./pages/RuntimePage";
import { UsagePage } from "./pages/UsagePage";
import { RolesPage } from "./pages/RolesPage";
import { useCurrentSession, useWorkspaceMeta } from "./api/queries";
import { useI18n, I18nProvider, type Locale } from "./i18n";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      staleTime: 10_000,
    },
  },
});

// ─── Root layout ──────────────────────────────────────────────────────────────

function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{children}</I18nProvider>
    </QueryClientProvider>
  );
}

// ─── App shell ────────────────────────────────────────────────────────────────

export function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const { locale, setLocale, t } = useI18n();
  const { data: currentSession } = useCurrentSession();
  const { data: workspace } = useWorkspaceMeta();
  const activeSessionId = currentSession?.id ?? null;
  const [sidebarConnection, setSidebarConnection] = useState<"idle" | "connecting" | "open" | "reconnecting" | "error">("idle");

  const handleConnectionChange = useCallback((value: "idle" | "connecting" | "open" | "reconnecting" | "error") => {
    setSidebarConnection(value);
  }, []);

  // Auto-enter chat once the current session is ready.
  useEffect(() => {
    if (activeSessionId && location.pathname === "/") {
      navigate("/chat");
    }
  }, [activeSessionId, navigate, location.pathname]);

  const isChat = location.pathname === "/chat" || location.pathname === "/";
  const isRuntime = location.pathname === "/runtime";
  const isUsage = location.pathname === "/usage";
  const isRoles = location.pathname === "/roles";

  useEffect(() => {
    if (!isChat) {
      setSidebarConnection("idle");
    }
  }, [isChat]);

  return (
    <div
      className="flex flex-col h-screen"
      style={{ backgroundColor: "var(--color-bg)", fontFamily: "var(--font-sans)" }}
    >
      {/* Top bar */}
      <header
        className="flex items-center justify-between px-6 py-3 shrink-0"
        style={{
          backgroundColor: "var(--color-surface)",
          borderBottom: "1px solid var(--color-border)",
        }}
      >
        {/* Brand */}
        <div className="flex items-center gap-3">
          <span
            className="text-base font-bold tracking-wide"
            style={{ color: "var(--color-accent)" }}
          >
            Sesame
          </span>
          {activeSessionId && (
            <span
              className="text-xs px-2 py-0.5 rounded-full"
              style={{
                backgroundColor: "var(--color-surface-2)",
                color: "var(--color-text-muted)",
                border: "1px solid var(--color-border)",
              }}
            >
              {workspace?.name || t("app.currentWorkspace")}
            </span>
          )}
        </div>

        <div className="flex items-center gap-4">
          <LanguageSwitcher locale={locale} onChange={setLocale} label={t("app.language")} />
          <nav className="flex gap-1">
          <NavTab
            to="/chat"
            active={isChat}
          >
            {t("nav.chat")}
          </NavTab>
          <NavTab
            to="/runtime"
            active={isRuntime}
          >
            {t("nav.runtime")}
          </NavTab>
          <NavTab
            to="/usage"
            active={isUsage}
          >
            {t("nav.usage")}
          </NavTab>
          <NavTab
            to="/roles"
            active={isRoles}
          >
            {t("nav.roles")}
          </NavTab>
          </nav>
        </div>
      </header>

      {/* Body */}
      <div className="flex flex-1 overflow-hidden">
        <Sidebar workspaceName={workspace?.name} workspaceRoot={workspace?.workspace_root} connection={sidebarConnection} />
        <main className="flex-1 flex flex-col overflow-hidden">
          {isChat && <ChatPage sessionId={activeSessionId ?? ""} onConnectionChange={handleConnectionChange} />}
          {isRuntime && <RuntimePage sessionId={activeSessionId ?? ""} />}
          {isUsage && <UsagePage sessionId={activeSessionId ?? undefined} />}
          {isRoles && <RolesPage />}
        </main>
      </div>
    </div>
  );
}

function LanguageSwitcher({
  locale,
  onChange,
  label,
}: {
  locale: Locale;
  onChange: (locale: Locale) => void;
  label: string;
}) {
  const options: Array<{ value: Locale; label: string }> = [
    { value: "en-US", label: "EN" },
    { value: "zh-CN", label: "中文" },
  ];

  return (
    <div className="flex items-center gap-2">
      <span className="text-xs" style={{ color: "var(--color-text-muted)" }}>
        {label}
      </span>
      <div
        className="flex rounded-lg p-1"
        style={{
          backgroundColor: "var(--color-surface-2)",
          border: "1px solid var(--color-border)",
        }}
      >
        {options.map((option) => {
          const active = option.value === locale;
          return (
            <button
              key={option.value}
              type="button"
              onClick={() => onChange(option.value)}
              className="rounded-md px-2.5 py-1 text-xs font-medium"
              style={{
                backgroundColor: active ? "var(--color-accent)" : "transparent",
                color: active ? "#fff" : "var(--color-text-muted)",
                border: "none",
                cursor: "pointer",
              }}
            >
              {option.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ─── NavTab ────────────────────────────────────────────────────────────────────

function NavTab({
  to,
  active,
  children,
}: {
  to: string;
  active: boolean;
  children: React.ReactNode;
}) {
  const [hovered, setHovered] = useState(false);
  return (
    <Link
      to={to}
      className="px-4 py-1.5 rounded-lg text-sm font-medium"
      style={{
        backgroundColor: active
          ? "var(--color-accent)"
          : hovered
          ? "rgba(255,255,255,0.06)"
          : "transparent",
        color: active ? "#fff" : "var(--color-text-muted)",
        textDecoration: "none",
        border: active ? "none" : "1px solid transparent",
        transition: "background-color 0.15s, color 0.15s",
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      {children}
    </Link>
  );
}

// ─── Router ────────────────────────────────────────────────────────────────────

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { path: "chat", element: null },
      { path: "runtime", element: null },
      { path: "usage", element: null },
      { path: "roles", element: null },
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
