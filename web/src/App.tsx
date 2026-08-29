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
import { SearchScreen } from "./screens/SearchScreen";

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
      <Route path="/lab" element={<RequireAuth><LabScreen /></RequireAuth>} />
      {/* The lab screens are per page; without an id they render their own
          page picker rather than 404ing, so these are real destinations. */}
      <Route path="/history" element={<RequireAuth><HistoryScreen /></RequireAuth>} />
      <Route path="/lab/trace" element={<RequireAuth><TraceScreen /></RequireAuth>} />
      <Route path="/lab/diff" element={<RequireAuth><DiffScreen /></RequireAuth>} />
      <Route path="*" element={<Navigate to="/pages" replace />} />
    </Routes>
  );
}

export default App;
