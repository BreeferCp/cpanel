import { useState, type FormEvent } from "react";
import { ApiError, api } from "@/lib/api";
import type { User } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { usePanel } from "@/state/panel";
import { Button } from "@/components/ui/Button";
import { Field, Input } from "@/components/ui/Form";
import { ErrorNote } from "@/components/ui/Layout";

interface LoginResponse {
  twofa_required?: boolean;
  challenge?: string;
  user?: User;
  csrf_token?: string;
}

export default function Login() {
  const { setSession } = useAuth();
  const panel = usePanel();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [challenge, setChallenge] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const isWhm = panel.panel === "whm";
  const isCp = panel.panel === "cpanel";
  const wrongPanel = error instanceof ApiError && error.code === "wrong_panel";
  const otherUrl = isWhm ? panel.cpanel_url : panel.whm_url;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const r = challenge
        ? await api.post<LoginResponse>("/auth/login/2fa", { challenge, code })
        : await api.post<LoginResponse>("/auth/login", { username, password });
      if (r.twofa_required && r.challenge) {
        setChallenge(r.challenge);
      } else if (r.user && r.csrf_token) {
        setSession(r.user, r.csrf_token);
      }
    } catch (err) {
      setError(err);
      if (err instanceof ApiError && err.code === "invalid_challenge") {
        setChallenge(null);
        setCode("");
      }
    } finally {
      setBusy(false);
    }
  }

  const title = challenge ? "Two-factor verification" : isWhm ? "Sign in to WHM" : isCp ? "Sign in to your hosting" : "Sign in";
  const subtitle = challenge
    ? "Enter the 6-digit code from your authenticator app, or a recovery code."
    : isWhm
      ? "For server administrators and resellers."
      : isCp
        ? "Manage your websites, email and databases."
        : "Use your hosting or administrator credentials.";

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg px-4">
      <div className="w-full max-w-[360px]">
        <div className="mb-6 flex items-center gap-2.5">
          <img src="/favicon.svg" alt="" className="size-8" />
          <span className="text-[20px] font-semibold tracking-tight">BreeferCP</span>
          {(isWhm || isCp) && (
            <span className="rounded-md border border-line bg-surface2 px-1.5 py-0.5 text-[11.5px] font-semibold uppercase tracking-wider text-muted">
              {isWhm ? "WHM" : "cPanel"}
            </span>
          )}
        </div>
        <form onSubmit={submit} className="space-y-4 rounded-xl border border-line bg-surface p-6 shadow-card">
          <div>
            <h1 className="text-[16px] font-semibold">{title}</h1>
            <p className="mt-0.5 text-[13px] text-muted">{subtitle}</p>
          </div>
          <ErrorNote error={error} />
          {wrongPanel && otherUrl && (
            <a href={otherUrl} className="block rounded-md border border-line bg-surface2 px-3 py-2 text-[13px] font-medium text-accent hover:underline">
              Go to the {isWhm ? "cPanel" : "WHM"} sign-in page →
            </a>
          )}
          {challenge ? (
            <Field label="Verification code">
              {(id) => (
                <Input
                  id={id}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  autoFocus
                  required
                  placeholder="123456"
                />
              )}
            </Field>
          ) : (
            <>
              <Field label="Username">
                {(id) => <Input id={id} value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />}
              </Field>
              <Field label="Password">
                {(id) => (
                  <Input id={id} type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
                )}
              </Field>
            </>
          )}
          <Button type="submit" variant="primary" className="w-full" loading={busy}>
            {challenge ? "Verify" : "Sign in"}
          </Button>
          {challenge && (
            <button type="button" className="w-full text-center text-[12.5px] text-muted hover:text-ink" onClick={() => setChallenge(null)}>
              Back to sign in
            </button>
          )}
        </form>
        {isCp && panel.webmail_url && (
          <p className="mt-4 text-center text-[12.5px] text-muted">
            Checking email?{" "}
            <a className="font-medium text-accent hover:underline" href={panel.webmail_url}>
              Open webmail
            </a>
          </p>
        )}
        {panel.hostname && <p className="mt-2 text-center text-[11.5px] text-subtle">{panel.hostname}</p>}
      </div>
    </div>
  );
}
