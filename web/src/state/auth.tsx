import { useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ApiError, api, setCsrf, setUnauthorizedHandler } from "@/lib/api";
import type { User } from "@/lib/types";

interface AuthState {
  user: User | null;
  loading: boolean;
  setSession: (user: User, csrf: string) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}

const Ctx = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const qc = useQueryClient();

  const refresh = useCallback(async () => {
    try {
      const r = await api.get<{ user: User; csrf_token: string }>("/me");
      setCsrf(r.csrf_token);
      setUser(r.user);
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) console.error(e);
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setUser(null);
      qc.clear();
    });
    void refresh();
  }, [refresh, qc]);

  const value = useMemo<AuthState>(
    () => ({
      user,
      loading,
      refresh,
      setSession: (u, csrf) => {
        setCsrf(csrf);
        setUser(u);
      },
      logout: async () => {
        try {
          await api.post("/auth/logout");
        } finally {
          setCsrf("");
          setUser(null);
          qc.clear();
        }
      },
    }),
    [user, loading, refresh, qc],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthState {
  const c = useContext(Ctx);
  if (!c) throw new Error("useAuth outside AuthProvider");
  return c;
}
