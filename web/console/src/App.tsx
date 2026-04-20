import { useState, useEffect } from "react";
import {
  createBrowserRouter,
  RouterProvider,
  useNavigate,
  useLocation,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Sidebar } from "./components/Sidebar";
import { ChatPage } from "./pages/ChatPage";
import { UsagePage } from "./pages/UsagePage";
import { RolesPage } from "./pages/RolesPage";
import { useSessions, useSelectSession } from "./api/queries";

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
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

// ─── App shell ────────────────────────────────────────────────────────────────

function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const { data } = useSessions();
  const selectSession = useSelectSession();

  // Auto-select first session on load
  useEffect(() => {
    if (!activeSessionId && data?.sessions.length) {
      const selected = data.sessions.find((s) => s.is_selected) ?? data.sessions[0];
      setActiveSessionId(selected.id);
      if (location.pathname === "/") {
        navigate("/chat");
      }
    }
  }, [data, activeSessionId, navigate, location.pathname]);

  function handleSelectSession(id: string) {
    setActiveSessionId(id);
    selectSession.mutate(id);
    navigate("/chat");
  }

  const isChat = location.pathname === "/chat" || location.pathname === "/";
  const isUsage = location.pathname === "/usage";
  const isRoles = location.pathname === "/roles";

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
              {data?.sessions.find((s) => s.id === activeSessionId)?.title || "Session"}
            </span>
          )}
        </div>

        {/* Nav tabs */}
        <nav className="flex gap-1">
          <NavTab
            to="/chat"
            active={isChat}
          >
            Chat
          </NavTab>
          <NavTab
            to="/usage"
            active={isUsage}
          >
            Usage
          </NavTab>
          <NavTab
            to="/roles"
            active={isRoles}
          >
            Roles
          </NavTab>
        </nav>
      </header>

      {/* Body */}
      <div className="flex flex-1 overflow-hidden">
        <Sidebar
          activeSessionId={activeSessionId}
          onSelectSession={handleSelectSession}
        />
        <main className="flex-1 flex flex-col overflow-hidden">
          {isChat && (
            <ChatPage
              sessionId={activeSessionId ?? ""}
              onSessionIdChange={handleSelectSession}
            />
          )}
          {isUsage && <UsagePage sessionId={activeSessionId ?? undefined} />}
          {isRoles && <RolesPage />}
        </main>
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
  return (
    <a
      href={to}
      className="px-4 py-1.5 rounded-lg text-sm font-medium transition-colors"
      style={{
        backgroundColor: active ? "var(--color-accent)" : "transparent",
        color: active ? "#fff" : "var(--color-text-muted)",
        textDecoration: "none",
        border: active ? "none" : "1px solid transparent",
      }}
    >
      {children}
    </a>
  );
}

// ─── Router ────────────────────────────────────────────────────────────────────

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { path: "chat", element: null },
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
