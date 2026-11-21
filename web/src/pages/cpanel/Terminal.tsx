import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { Plug, PlugZap } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useGet } from "@/lib/hooks";
import type { Account } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Badge, Card, EmptyState, Loading, PageHeader } from "@/components/ui/Layout";

type State = "connecting" | "open" | "closed";

export default function Terminal() {
  const aid = useAccountId();
  const acct = useGet<Account>(`/accounts/${aid}`);
  const host = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<State>("closed");
  const [note, setNote] = useState("");
  const [session, setSession] = useState(0);
  const enabled = acct.data?.shell === "full";

  useEffect(() => {
    if (!enabled || !host.current || session === 0) return;
    const term = new XTerm({
      cursorBlink: true,
      fontFamily: getComputedStyle(document.body).getPropertyValue("--font-mono") || "monospace",
      fontSize: 13,
      theme: { background: "#0d1015", foreground: "#e6e9ee", cursor: "#6b8cff" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host.current);
    fit.fit();
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${proto}://${location.host}/api/v1/accounts/${aid}/terminal`);
    ws.binaryType = "arraybuffer";
    setState("connecting");
    const enc = new TextEncoder();
    const resize = () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
    };
    ws.onopen = () => {
      setState("open");
      setNote("");
      resize();
      term.focus();
    };
    ws.onmessage = (e) => term.write(new Uint8Array(e.data as ArrayBuffer));
    ws.onclose = (e) => {
      setState("closed");
      setNote(e.reason || "Disconnected.");
      term.write("\r\n\x1b[90m[session closed]\x1b[0m\r\n");
    };
    const sub = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("resize", resize);
      sub.dispose();
      ws.close();
      term.dispose();
    };
  }, [aid, enabled, session]);

  if (acct.isLoading) return <Loading />;
  return (
    <>
      <PageHeader
        title="Terminal"
        description="A shell on the server as your account user, inside your plan's resource limits."
        actions={
          enabled && (
            <>
              <Badge tone={state === "open" ? "ok" : state === "connecting" ? "warn" : "neutral"}>
                {state === "open" ? "Connected" : state === "connecting" ? "Connecting…" : "Not connected"}
              </Badge>
              <Button variant={state === "open" ? "secondary" : "primary"} icon={state === "open" ? Plug : PlugZap} onClick={() => setSession((s) => s + 1)}>
                {state === "open" ? "Reconnect" : "Connect"}
              </Button>
            </>
          )
        }
      />
      {!enabled ? (
        <Card>
          <EmptyState
            icon={PlugZap}
            title="Shell access is not enabled"
            description="Your hosting provider can enable full shell access for this account if your plan includes it."
          />
        </Card>
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-[#0d1015] p-2 shadow-card">
          {session === 0 ? (
            <div className="flex h-[60vh] items-center justify-center text-[13px] text-[#9ba5b4]">Press Connect to open a session.</div>
          ) : (
            <div ref={host} className="h-[60vh]" />
          )}
        </div>
      )}
      {note && state === "closed" && <p className="mt-2 text-[12.5px] text-muted">{note}</p>}
    </>
  );
}
