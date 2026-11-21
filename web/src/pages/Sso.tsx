import { useEffect, useRef, useState } from "react";
import { Navigate } from "react-router-dom";
import { api } from "@/lib/api";
import type { User } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { ErrorNote, Spinner } from "@/components/ui/Layout";

// Redeems a one-time sign-in token from the URL fragment (never sent to the
// server in a request line, so it stays out of logs and Referer headers).
export default function Sso() {
  const { setSession, user } = useAuth();
  const [error, setError] = useState<unknown>(null);
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const token = new URLSearchParams(location.hash.slice(1)).get("token");
    history.replaceState(null, "", location.pathname); // scrub the token from the address bar
    if (!token) {
      setError(new Error("This sign-in link is incomplete."));
      return;
    }
    api
      .post<{ user: User; csrf_token: string }>("/auth/sso", { token })
      .then((r) => setSession(r.user, r.csrf_token))
      .catch(setError);
  }, [setSession]);

  if (user) return <Navigate to={user.role === "user" ? "/hosting" : "/"} replace />;
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm text-center">
        {error ? <ErrorNote error={error} /> : <Spinner className="mx-auto size-6" />}
      </div>
    </div>
  );
}
