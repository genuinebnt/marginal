import type { ReactElement } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./auth/AuthContext";
import { AuthPage } from "./screens/AuthPage";
import { DashboardScreen } from "./screens/DashboardScreen";
import { EditorScreen } from "./screens/EditorScreen";

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
      <Route path="*" element={<Navigate to="/pages" replace />} />
    </Routes>
  );
}

export default App;
