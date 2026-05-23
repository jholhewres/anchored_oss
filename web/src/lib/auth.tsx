import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";

import { api, clearToken, getToken, setToken } from "@/lib/api";
import type { Me } from "@/lib/types";

interface AuthContextValue {
  me: Me | null;
  loading: boolean;
  error: string | null;
  login: (email: string, password: string) => Promise<Me>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState<boolean>(() => Boolean(getToken()));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const token = getToken();
    if (!token) {
      setLoading(false);
      return;
    }
    let active = true;
    api
      .getMe()
      .then((data) => {
        if (active) setMe(data);
      })
      .catch(() => {
        if (active) {
          clearToken();
          setMe(null);
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    setError(null);
    try {
      const res = await api.login(email, password);
      setToken(res.api_key);
      const data = await api.getMe();
      setMe(data);
      return data;
    } catch (e) {
      clearToken();
      setMe(null);
      const msg = e instanceof Error ? e.message : "invalid credentials";
      setError(msg);
      throw e;
    }
  }, []);

  const logout = useCallback(() => {
    clearToken();
    setMe(null);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({ me, loading, error, login, logout }),
    [me, loading, error, login, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const { me, loading } = useAuth();
  const location = useLocation();
  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center text-muted-foreground">
        Loading…
      </div>
    );
  }
  if (!me) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }
  return <>{children}</>;
}
