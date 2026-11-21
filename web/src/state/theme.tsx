import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type ThemeChoice = "light" | "dark" | "system";

interface ThemeState {
  choice: ThemeChoice;
  setChoice: (t: ThemeChoice) => void;
}

const Ctx = createContext<ThemeState | null>(null);
const KEY = "breefer-theme";

function resolve(c: ThemeChoice): "light" | "dark" {
  if (c === "system") return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  return c;
}

function read(): ThemeChoice {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    /* storage blocked */
  }
  return "light"; // light is the default theme
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [choice, setChoiceState] = useState<ThemeChoice>(read);

  useEffect(() => {
    document.documentElement.dataset.theme = resolve(choice);
    if (choice !== "system") return;
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const on = () => (document.documentElement.dataset.theme = resolve("system"));
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, [choice]);

  const setChoice = useCallback((t: ThemeChoice) => {
    setChoiceState(t);
    try {
      localStorage.setItem(KEY, t);
    } catch {
      /* ignore */
    }
  }, []);

  const value = useMemo(() => ({ choice, setChoice }), [choice, setChoice]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useTheme(): ThemeState {
  const c = useContext(Ctx);
  if (!c) throw new Error("useTheme outside ThemeProvider");
  return c;
}
