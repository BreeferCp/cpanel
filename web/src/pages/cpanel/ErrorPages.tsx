import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, ErrorPage } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { DomainPicker } from "@/components/ui/DomainPicker";
import { Textarea } from "@/components/ui/Form";
import { Badge, Card, Loading, PageHeader } from "@/components/ui/Layout";

const names: Record<number, string> = {
  400: "Bad request",
  401: "Authorization required",
  403: "Forbidden",
  404: "Not found",
  500: "Internal server error",
  502: "Bad gateway",
  503: "Service unavailable",
};

const template = (code: number) => `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${code} ${names[code]}</title>
  <style>
    body { font: 16px/1.5 system-ui, sans-serif; display: grid; place-items: center; min-height: 100vh; margin: 0; color: #333; }
    h1 { font-size: 48px; margin: 0; }
  </style>
</head>
<body>
  <main>
    <h1>${code}</h1>
    <p>${names[code]}.</p>
    <p><a href="/">Back to the home page</a></p>
  </main>
</body>
</html>
`;

export default function ErrorPages() {
  const aid = useAccountId();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [domainId, setDomainId] = useState("");
  const key = `/accounts/${aid}/domains/${domainId}/error-pages`;
  const q = useGet<ErrorPage[]>(key, !!domainId);
  const [code, setCode] = useState(404);
  const [draft, setDraft] = useState<Record<number, string>>({});

  const page = q.data?.find((p) => p.code === code);
  const value = draft[code] ?? (page?.custom ? page.content : template(code));

  const save = useMutate(() => api.put(`${key}/${code}`, { content: value }), {
    invalidate: [key],
    success: `Error page ${code} saved`,
    onSuccess: () => setDraft((d) => ({ ...d, [code]: undefined as unknown as string })),
  });
  const reset = useMutate(() => api.del(`${key}/${code}`), { invalidate: [key], success: "Restored the default page" });

  return (
    <>
      <PageHeader title="Error Pages" description="Show your own branded page instead of the server default when something goes wrong." />
      <div className="mb-4">
        <DomainPicker domains={domains.data} value={domainId} onChange={setDomainId} />
      </div>
      {q.isLoading || !q.data ? (
        <Loading />
      ) : (
        <div className="grid gap-5 lg:grid-cols-[220px_1fr]">
          <nav className="space-y-1" aria-label="Status codes">
            {q.data.map((p) => (
              <button
                key={p.code}
                onClick={() => setCode(p.code)}
                className={`flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-[13px] ${p.code === code ? "bg-accent-soft font-medium text-accent" : "text-ink hover:bg-surface2"}`}
              >
                <span>
                  {p.code} <span className="text-muted">{names[p.code]}</span>
                </span>
                {p.custom && <Badge tone="ok">Custom</Badge>}
              </button>
            ))}
          </nav>
          <Card
            title={`${code} ${names[code]}`}
            description={page?.custom ? "Custom page active." : "Using the server default. Save to activate this page."}
            actions={
              <>
                {page?.custom && (
                  <Button size="sm" variant="ghost" loading={reset.isPending} onClick={() => reset.mutate()}>
                    Restore default
                  </Button>
                )}
                <Button size="sm" variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
                  Save page
                </Button>
              </>
            }
          >
            <Textarea
              aria-label={`HTML for the ${code} page`}
              className="h-[420px] w-full resize-y whitespace-pre text-[12.5px] leading-5"
              spellCheck={false}
              value={value}
              onChange={(e) => setDraft({ ...draft, [code]: e.target.value })}
            />
          </Card>
        </div>
      )}
    </>
  );
}
