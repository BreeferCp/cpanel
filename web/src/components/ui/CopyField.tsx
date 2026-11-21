import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { IconButton } from "./Button";

// Read-only value with a copy button, used for tokens, DNS values and secrets.
export function CopyField({ value, mono = true }: { value: string; mono?: boolean }) {
  const [done, setDone] = useState(false);
  return (
    <div className="flex items-center gap-1 rounded-md border border-strong bg-surface2 pl-2.5 pr-1">
      <input
        readOnly
        value={value}
        onFocus={(e) => e.currentTarget.select()}
        className={`h-8 min-w-0 flex-1 bg-transparent text-[12.5px] text-ink outline-none ${mono ? "font-mono" : ""}`}
        aria-label="Value"
      />
      <IconButton
        icon={done ? Check : Copy}
        label={done ? "Copied" : "Copy"}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setDone(true);
            setTimeout(() => setDone(false), 1500);
          } catch {
            /* clipboard unavailable: the value is selectable */
          }
        }}
      />
    </div>
  );
}
