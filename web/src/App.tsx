import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";

import { AppLayout } from "@/components/layout/AppLayout";
import { RequireAuth } from "@/lib/auth";
import { api, getToken } from "@/lib/api";

import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import { OnboardingPage } from "@/pages/OnboardingPage";
import { InviteAcceptPage } from "@/pages/InviteAcceptPage";
import { OverviewPage } from "@/pages/OverviewPage";
import { ProjectsPage } from "@/pages/ProjectsPage";
import { ProjectDetailPage } from "@/pages/ProjectDetailPage";
import { DevelopersPage } from "@/pages/DevelopersPage";
import { APIKeysPage } from "@/pages/APIKeysPage";
import { AuditPage } from "@/pages/AuditPage";
import { HealthPage } from "@/pages/HealthPage";

function RootRouter() {
  const [bootstrapped, setBootstrapped] = useState<boolean | null>(null);

  useEffect(() => {
    api.getBootstrapStatus()
      .then(res => setBootstrapped(res.bootstrapped))
      .catch(() => setBootstrapped(true)); // on error, assume bootstrapped → show login
  }, []);

  if (bootstrapped === null) {
    return (
      <div style={{
        display: "flex", alignItems: "center", justifyContent: "center",
        height: "100vh", color: "var(--text-dim)", fontFamily: "var(--font-mono)", fontSize: 13,
      }}>
        Loading…
      </div>
    );
  }

  if (!bootstrapped) return <Navigate to="/onboarding" replace />;
  if (!getToken()) return <Navigate to="/login" replace />;
  return <Navigate to="/dashboard" replace />;
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<RootRouter />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/onboarding" element={<OnboardingPage />} />
      <Route path="/invite/:token" element={<InviteAcceptPage />} />
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
        <Route path="/developers" element={<DevelopersPage />} />
        <Route path="/accounts" element={<Navigate to="/developers" replace />} />
        <Route path="/api-keys" element={<APIKeysPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/health" element={<HealthPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
