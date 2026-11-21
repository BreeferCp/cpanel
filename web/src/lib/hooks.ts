import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { useToast } from "@/components/ui/Toast";

/** GET a JSON resource; the path doubles as the cache key. */
export function useGet<T>(path: string, enabled = true, refetchInterval?: (data: T | undefined) => number | false) {
  return useQuery({
    queryKey: [path],
    queryFn: () => api.get<T>(path),
    enabled,
    refetchInterval: refetchInterval ? (q) => refetchInterval(q.state.data as T | undefined) : undefined,
  });
}

/**
 * Mutation that invalidates the given cache keys on success and reports the
 * outcome through a toast, so pages stay free of boilerplate.
 */
export function useMutate<V = void, R = unknown>(
  fn: (v: V) => Promise<R>,
  opts: { invalidate?: string[]; success?: string | ((r: R, v: V) => string); onSuccess?: (r: R, v: V) => void } = {},
) {
  const qc = useQueryClient();
  const toast = useToast();
  return useMutation({
    mutationFn: fn,
    onSuccess: async (r, v) => {
      await Promise.all((opts.invalidate ?? []).map((k) => qc.invalidateQueries({ queryKey: [k] })));
      if (opts.success) toast.success(typeof opts.success === "function" ? opts.success(r, v) : opts.success);
      opts.onSuccess?.(r, v);
    },
    onError: (e) => toast.error(e),
  });
}
