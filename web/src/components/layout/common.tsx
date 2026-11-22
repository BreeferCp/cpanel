import { LogOut, Monitor, Moon, Sun } from "lucide-react";
import { useAuth } from "@/state/auth";
import { useTheme, type ThemeChoice } from "@/state/theme";
import { IconButton } from "../ui/Button";

export function ThemeToggle() {
  const { choice, setChoice } = useTheme();
  const next: Record<ThemeChoice, ThemeChoice> = { light: "dark", dark: "system", system: "light" };
  const Icon = choice === "light" ? Sun : choice === "dark" ? Moon : Monitor;
  return <IconButton icon={Icon} label={`Theme: ${choice} (click to change)`} onClick={() => setChoice(next[choice])} />;
}

export function UserChip({ subtitle }: { subtitle?: string }) {
  const { user, logout } = useAuth();
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[12px] font-semibold uppercase text-accent">
        {user?.username.slice(0, 1)}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] font-medium text-ink">{user?.username}</div>
        <div className={subtitle ? "truncate text-[11.5px] text-muted" : "truncate text-[11.5px] capitalize text-muted"}>{subtitle ?? user?.role}</div>
      </div>
      <ThemeToggle />
      <IconButton icon={LogOut} label="Sign out" onClick={() => void logout()} />
    </div>
  );
}

export function PanelMark({ label }: { label: string }) {
  return (
    <span className="rounded border border-line bg-surface2 px-1 py-px text-[10.5px] font-semibold uppercase tracking-wider text-muted">{label}</span>
  );
}
