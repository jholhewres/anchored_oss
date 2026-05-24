import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { I } from "@/ds/icons";

type Variant = "default" | "success" | "error" | "info";

interface Toast {
  id: number;
  title: string;
  description?: string;
  variant: Variant;
}

interface ToastContextValue {
  push: (t: Omit<Toast, "id">) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const variantStyles: Record<Variant, { icon: ReactNode; color: string }> = {
  default: { icon: <I.info size={14} />, color: "var(--text-muted)" },
  info: { icon: <I.info size={14} />, color: "var(--info)" },
  success: { icon: <I.check size={14} />, color: "var(--ok)" },
  error: { icon: <I.alert size={14} />, color: "var(--err)" },
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const push = useCallback((t: Omit<Toast, "id">) => {
    setToasts((cur) => [...cur, { id: Date.now() + Math.random(), ...t }]);
  }, []);

  const dismiss = useCallback((id: number) => {
    setToasts((cur) => cur.filter((t) => t.id !== id));
  }, []);

  useEffect(() => {
    if (toasts.length === 0) return;
    const timer = setTimeout(() => {
      setToasts((cur) => cur.slice(1));
    }, 5_000);
    return () => clearTimeout(timer);
  }, [toasts]);

  const value = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        style={{
          position: "fixed",
          bottom: 16,
          right: 16,
          zIndex: 100,
          display: "flex",
          flexDirection: "column",
          gap: 8,
          maxWidth: 400,
          pointerEvents: "none",
        }}
      >
        {toasts.map((t) => {
          const vs = variantStyles[t.variant] || variantStyles.default;
          return (
            <div
              key={t.id}
              style={{
                pointerEvents: "auto",
                display: "flex",
                alignItems: "flex-start",
                gap: 12,
                padding: "10px 14px",
                background: "var(--bg-2)",
                border: "1px solid var(--border)",
                borderRadius: "var(--radius)",
                boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
              }}
            >
              <span style={{ color: vs.color, marginTop: 1, display: "inline-flex" }}>
                {vs.icon}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 13, fontWeight: 500 }}>{t.title}</div>
                {t.description && (
                  <div
                    style={{
                      fontSize: 12,
                      color: "var(--text-muted)",
                      marginTop: 2,
                    }}
                  >
                    {t.description}
                  </div>
                )}
              </div>
              <button
                type="button"
                onClick={() => dismiss(t.id)}
                aria-label="Dismiss"
                style={{
                  background: "transparent",
                  border: 0,
                  color: "var(--text-dim)",
                  cursor: "pointer",
                  padding: 2,
                  display: "inline-flex",
                }}
              >
                <I.x size={14} />
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
