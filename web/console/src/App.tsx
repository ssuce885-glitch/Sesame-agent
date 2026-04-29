import { useCallback, useEffect, useState } from "react";
import {
  createBrowserRouter,
  RouterProvider,
  useNavigate,
  useLocation,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Sidebar } from "./components/Sidebar";
import { ChatPage } from "./pages/ChatPage";
import { RuntimePage } from "./pages/RuntimePage";
import { UsagePage } from "./pages/UsagePage";
import { RolesPage } from "./pages/RolesPage";
import { useCurrentSession, useWorkspaceMeta } from "./api/queries";
import { useI18n, I18nProvider, type Locale } from "./i18n";
import { Globe } from "./components/Icon";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      staleTime: 10_000,
    },
  },
});

function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{children}</I18nProvider>
    </QueryClientProvider>
  );
}

export function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const { locale, setLocale, t } = useI18n();
  const { data: currentSession } = useCurrentSession();
  const { data: workspace } = useWorkspaceMeta();
  const activeSessionId = currentSession?.id ?? null;
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

  return (
    <div className="flex h-screen" style={{ backgroundColor: "var(--color-bg)" }}>
      <Sidebar
        workspaceName={workspace?.name}
        workspaceRoot={workspace?.workspace_root}
        connection={sidebarConnection}
        activePath={activePath}
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
              {workspace?.name || "Sesame"}
            </span>
            {workspace?.workspace_root && (
              <>
                <span style={{ color: "var(--color-border-strong)" }}>/</span>
                <span
                  className="text-xs truncate max-w-[200px]"
                  style={{ color: "var(--color-text-tertiary)" }}
                >
                  {workspace.workspace_root.split("/").pop()}
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
          {activePath === "/runtime" && (
            <RuntimePage sessionId={activeSessionId ?? ""} />
          )}
          {activePath === "/usage" && (
            <UsagePage sessionId={activeSessionId ?? undefined} />
          )}
          {activePath === "/roles" && <RolesPage />}
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
