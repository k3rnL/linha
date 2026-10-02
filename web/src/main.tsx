import { StrictMode, useEffect } from "react";
import { createRoot } from "react-dom/client";
import {
  BrowserRouter,
  Link,
  NavLink,
  Navigate,
  Route,
  Routes,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  Activity,
  LayoutDashboard,
  ArrowUpRight,
  Boxes,
  ListTodo,
  Network,
  LogOut,
  Cable,
} from "lucide-react";
import { api, APIError, setCSRF, useData } from "./api";
import {
  ContextDetail,
  Contexts,
  NewRequest,
  RequestDetail,
  Requests,
  Servers,
} from "./pages";
import { Button } from "./components/ui/button";
import { ErrorMessage } from "./components/common";
import type { RuntimeConfig, Session } from "./types";
import "./style.css";
import { Overview } from "./overview";
const client = new QueryClient({
  defaultOptions: { queries: { staleTime: 3000, refetchOnWindowFocus: true } },
});
function App() {
  const config = useData<RuntimeConfig>("/ui/config", false);
  const session = useData<Session>("/v1/admin/session");
  useEffect(() => {
    setCSRF(session.data?.csrf);
  }, [session.data]);
  const login = config.data?.loginURL ?? "/ui/oidc/login";
  if (session.isPending)
    return (
      <main className="login">
        <Cable size={36} />
        <h1>Linha</h1>
        <p>Connecting to the operations console…</p>
      </main>
    );
  if (session.error) {
    const unauthorized =
      session.error instanceof APIError && session.error.status === 401;
    return (
      <main className="login">
        <Cable size={36} />
        <h1>Linha Operations</h1>
        <p>
          {unauthorized
            ? "Sign in with an administrator account."
            : "Administrator access is unavailable."}
        </p>
        {!unauthorized && <ErrorMessage error={session.error} />}
        <Button asChild>
          <a href={login}>{unauthorized ? "Sign in" : "Use another account"}</a>
        </Button>
      </main>
    );
  }
  if (!session.data) return null;
  const user = session.data;
  const operator = user.role === "operator";
  return (
    <div className="app">
      <aside className="sidebar">
        <Link className="brand" to="/">
          <Cable size={29} />
          <span>
            Linha<small>Operations console</small>
          </span>
        </Link>
        <div className="nav-label">WORKSPACE</div>
        <nav>
          <NavLink to="/" end>
            <LayoutDashboard size={18} />
            Overview
          </NavLink>
          <NavLink to="/contexts">
            <Boxes size={18} />
            Contexts
          </NavLink>
          <NavLink to="/requests">
            <ListTodo size={18} />
            Requests
          </NavLink>
          <NavLink to="/servers">
            <Network size={18} />
            Servers
          </NavLink>
          {config.data?.grafanaURL && (
            <a
              href={config.data.grafanaURL}
              target="_blank"
              rel="noopener noreferrer"
            >
              <Activity size={18} />
              Grafana
              <ArrowUpRight size={15} />
            </a>
          )}
        </nav>
        <div className="sidebar-footer">
          <span className="role">{user.role} access</span>
          <small title={user.actor}>{user.actor}</small>
          {config.data?.securityEnabled && (
            <button
              onClick={async () => {
                try {
                  await api("/v1/admin/logout", { method: "POST", body: "{}" });
                  client.clear();
                  window.location.assign("/ui/");
                } catch {
                  void session.refetch();
                }
              }}
            >
              <LogOut size={16} />
              Sign out
            </button>
          )}
        </div>
      </aside>
      <main className="main">
        <div className="topbar">
          <span>
            <span className="dot" /> Administrative workspace
          </span>
          <span>Refreshes while visible · every 5s</span>
        </div>
        <div className="content">
          <Routes>
            <Route path="/" element={<Overview operator={operator} />} />
            <Route
              path="/contexts"
              element={<Contexts operator={operator} />}
            />
            <Route
              path="/contexts/:id"
              element={<ContextDetail operator={operator} />}
            />
            <Route
              path="/requests"
              element={<Requests operator={operator} />}
            />
            <Route
              path="/requests/new"
              element={<NewRequest session={user} />}
            />
            <Route
              path="/requests/:id"
              element={<RequestDetail operator={operator} />}
            />
            <Route path="/servers" element={<Servers />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter basename="/ui">
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
