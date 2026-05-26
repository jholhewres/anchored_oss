import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import { AppLayout } from "@/components/layout/AppLayout";
import { RedirectIfAuth, RequireAuth } from "@/lib/auth";
import { getMode } from "@/lib/api";
import type { Mode } from "@/lib/types";
import { LoginPage } from "@/pages/LoginPage";
import { LandingPage } from "@/pages/LandingPage";
import { RegisterPage } from "@/pages/RegisterPage";
import { OverviewPage } from "@/pages/OverviewPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { ProjectDetailPage } from "@/pages/ProjectDetailPage";
import { AccountsPage } from "@/pages/AccountsPage";
import { TeamsPage } from "@/pages/TeamsPage";
import { TeamDetailPage } from "@/pages/TeamDetailPage";
import { APIKeysPage } from "@/pages/APIKeysPage";
import { AuditPage } from "@/pages/AuditPage";
import { HealthPage } from "@/pages/HealthPage";

export function App() {
  const [mode, setMode] = useState<Mode>("selfhosted");

  useEffect(() => {
    getMode().then((res) => setMode(res.mode)).catch(() => {});
  }, []);

  if (mode === "cloud") {
    return (
      <Routes>
        <Route
          path="/"
          element={
            <RedirectIfAuth>
              <LandingPage />
            </RedirectIfAuth>
          }
        />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route
          element={
            <RequireAuth>
              <AppLayout />
            </RequireAuth>
          }
        >
          <Route path="/dashboard" element={<OverviewPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/:id" element={<ProjectDetailPage />} />
          <Route path="/accounts" element={<AccountsPage />} />
          <Route path="/teams" element={<TeamsPage />} />
          <Route path="/teams/:id" element={<TeamDetailPage />} />
          <Route path="/api-keys" element={<APIKeysPage />} />
          <Route path="/audit" element={<AuditPage />} />
          <Route path="/health" element={<HealthPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    );
  }

  return (
    <Routes>
      <Route
        path="/"
        element={
          <RedirectIfAuth>
            <LandingPage />
          </RedirectIfAuth>
        }
      />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route path="/dashboard" element={<OverviewPage />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/projects/:id" element={<ProjectDetailPage />} />
        <Route path="/accounts" element={<AccountsPage />} />
        <Route path="/teams" element={<TeamsPage />} />
        <Route path="/teams/:id" element={<TeamDetailPage />} />
        <Route path="/api-keys" element={<APIKeysPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/health" element={<HealthPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
