import { useState, type FormEvent } from "react";
import { ApiError, api } from "@/lib/api";
import type { User } from "@/lib/types";
import { useAuth } from "@/state/auth";
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
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [challenge, setChallenge] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

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

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg px-4">
      <div className="w-full max-w-[360px]">
        <div className="mb-6 flex items-center gap-2.5">
          <img src="/favicon.svg" alt="" className="size-8" />
          <span className="text-[20px] font-semibold tracking-tight">BreeferCP</span>
        </div>
        <form onSubmit={submit} className="space-y-4 rounded-xl border border-line bg-surface p-6 shadow-card">
          <div>
            <h1 className="text-[16px] font-semibold">{challenge ? "Two-factor verification" : "Sign in"}</h1>
            <p className="mt-0.5 text-[13px] text-muted">
              {challenge ? "Enter the 6-digit code from your authenticator app, or a recovery code." : "Use your hosting or administrator credentials."}
            </p>
          </div>
          <ErrorNote error={error} />
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
                {(id) => (
                  <Input id={id} value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />
                )}
              </Field>
              <Field label="Password">
                {(id) => (
                  <Input
                    id={id}
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="current-password"
                    required
                  />
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
      </div>
    </div>
  );
}
