import type { ReactElement } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth/AuthContext";
import { AuthPage } from "./screens/AuthPage";
import { DashboardScreen } from "./screens/DashboardScreen";
import { EditorScreen } from "./screens/EditorScreen";
import { GraphScreen } from "./screens/GraphScreen";
import { GraphAlgorithmsScreen } from "./screens/GraphAlgorithmsScreen";
import { FactsScreen } from "./screens/FactsScreen";
import { HistoryScreen } from "./screens/HistoryScreen";
import { TraceScreen } from "./screens/TraceScreen";
import { DiffScreen } from "./screens/DiffScreen";
import { TopicsScreen } from "./screens/TopicsScreen";
import { LabScreen } from "./screens/LabScreen";
import { ReaderScreen } from "./screens/ReaderScreen";
import { SearchScreen } from "./screens/SearchScreen";
import { NotificationsScreen } from "./screens/NotificationsScreen";
import { DiscoverScreen } from "./screens/DiscoverScreen";
import { SeriesScreen } from "./screens/SeriesScreen";
import { TrashScreen } from "./screens/TrashScreen";
import { NotFoundScreen } from "./screens/NotFoundScreen";
import { LabCompilerScreen } from "./screens/LabCompilerScreen";
import { LabAnalyticsScreen } from "./screens/LabAnalyticsScreen";

function RequireAuth({ children }: { children: ReactElement }) {
  const { session } = useAuth();
  if (!session) return <Navigate to="/login" replace />;
  return children;
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<AuthPage />} />
      <Route
        path="/pages"
        element={
          <RequireAuth>
            <DashboardScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/pages/:id"
        element={
          <RequireAuth>
            <EditorScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/graph"
        element={
          <RequireAuth>
            <GraphScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/graph/algorithms"
        element={
          <RequireAuth>
            <GraphAlgorithmsScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/facts"
        element={
          <RequireAuth>
            <FactsScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/pages/:id/history"
        element={
          <RequireAuth>
            <HistoryScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/pages/:id/trace"
        element={
          <RequireAuth>
            <TraceScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/pages/:id/diff"
        element={
          <RequireAuth>
            <DiffScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/search"
        element={
          <RequireAuth>
            <SearchScreen />
          </RequireAuth>
        }
      />
      <Route
        path="/topics"
        element={
          <RequireAuth>
            <TopicsScreen />
          </RequireAuth>
        }
      />
      <Route path="/trash" element={<RequireAuth><TrashScreen /></RequireAuth>} />
      <Route path="/series" element={<RequireAuth><SeriesScreen /></RequireAuth>} />
      <Route path="/series/:id" element={<RequireAuth><SeriesScreen /></RequireAuth>} />
      <Route path="/discover" element={<RequireAuth><DiscoverScreen /></RequireAuth>} />
      <Route path="/discover/:id" element={<RequireAuth><DiscoverScreen /></RequireAuth>} />
      <Route path="/notifications" element={<RequireAuth><NotificationsScreen /></RequireAuth>} />
      <Route path="/lab" element={<RequireAuth><LabScreen /></RequireAuth>} />
      <Route path="/read" element={<RequireAuth><ReaderScreen /></RequireAuth>} />
      <Route path="/read/:id" element={<RequireAuth><ReaderScreen /></RequireAuth>} />
      {/* The lab screens are per page; without an id they render their own
          page picker rather than 404ing, so these are real destinations. */}
      <Route path="/history" element={<RequireAuth><HistoryScreen /></RequireAuth>} />
      <Route path="/lab/compiler" element={<RequireAuth><LabCompilerScreen /></RequireAuth>} />
      <Route path="/lab/analytics" element={<RequireAuth><LabAnalyticsScreen /></RequireAuth>} />
      <Route path="/lab/trace" element={<RequireAuth><TraceScreen /></RequireAuth>} />
      <Route path="/lab/diff" element={<RequireAuth><DiffScreen /></RequireAuth>} />
      {/* A wrong URL is not a redirect. Bouncing to /pages silently discards
          what the person actually asked for, and § 24e's whole argument is
          that a missing page is a dangling state offering an action — not an
          error, and certainly not a shrug that loses the name they typed. */}
      <Route path="*" element={<RequireAuth><NotFoundScreen /></RequireAuth>} />
    </Routes>
  );
}

export default App;
