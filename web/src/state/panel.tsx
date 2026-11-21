import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, type ReactNode } from "react";
import { api } from "@/lib/api";
import type { PanelInfo } from "@/lib/types";

const Ctx = createContext<PanelInfo | null>(null);

/**
 * Loads which panel this origin serves (WHM on :2087, cPanel on :2083) before
 * sign-in, so the login screen and layout match the port.
 */
export function PanelProvider({ children }: { children: ReactNode }) {
  const q = useQuery({ queryKey: ["/panel"], queryFn: () => api.get<PanelInfo>("/panel"), staleTime: 5 * 60_000 });
  const info = q.data ?? { panel: "both" as const, whm_url: "", cpanel_url: "", webmail_url: "", hostname: "" };
  return <Ctx.Provider value={info}>{children}</Ctx.Provider>;
}

export function usePanel(): PanelInfo {
  const v = useContext(Ctx);
  if (!v) throw new Error("usePanel outside PanelProvider");
  return v;
}
