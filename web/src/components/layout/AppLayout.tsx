import { Outlet } from "react-router-dom";

import { Header } from "@/components/layout/Header";
import { Sidebar } from "@/components/layout/Sidebar";

export function AppLayout() {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "240px 1fr",
        height: "100vh",
        minHeight: "100vh",
      }}
    >
      <Sidebar />
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          minWidth: 0,
          background: "var(--bg)",
        }}
      >
        <Header />
        <main style={{ padding: "28px 36px 60px", flex: 1, overflow: "auto" }}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
