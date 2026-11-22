import clsx from "clsx";
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight, Search, type LucideIcon } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Button } from "./Button";
import { Checkbox, Input, Select } from "./Form";
import { EmptyState, ErrorNote, Loading, useColumnLabels } from "./Layout";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  /** Value used for sorting; the column is sortable when given. */
  sort?: (row: T) => string | number;
  className?: string;
  /** Hidden below the sm breakpoint. */
  hideSm?: boolean;
}

export interface BulkAction<T> {
  label: string;
  icon?: LucideIcon;
  danger?: boolean;
  onRun: (rows: T[]) => void;
}

function StackTable({ children }: { children: ReactNode }) {
  const ref = useColumnLabels();
  return (
    <div className="relative overflow-x-auto">
      <table ref={ref} className="stack w-full border-collapse text-left text-[13px]">
        {children}
      </table>
    </div>
  );
}

/**
 * The table every tool page uses: search, column sorting, paging, row
 * selection with bulk actions, and consistent loading / error / empty states.
 */
export function DataTable<T>({
  rows,
  columns,
  rowKey,
  loading,
  error,
  search,
  searchPlaceholder = "Search",
  empty,
  bulk,
  actions,
  toolbar,
  pageSize: initialSize = 25,
  initialSort,
  rowClassName,
}: {
  rows: T[] | undefined;
  columns: Column<T>[];
  rowKey: (row: T) => string;
  loading?: boolean;
  error?: unknown;
  /** Text searched by the filter box; omit to hide the box. */
  search?: (row: T) => string;
  searchPlaceholder?: string;
  empty: { icon: LucideIcon; title: string; description?: string; action?: ReactNode };
  bulk?: BulkAction<T>[];
  /** Per-row actions, right-aligned in the last column. */
  actions?: (row: T) => ReactNode;
  toolbar?: ReactNode;
  pageSize?: number;
  initialSort?: { key: string; dir: "asc" | "desc" };
  rowClassName?: (row: T) => string | undefined;
}) {
  const [q, setQ] = useState("");
  const [sort, setSort] = useState(initialSort);
  const [page, setPage] = useState(0);
  const [size, setSize] = useState(initialSize);
  const [picked, setPicked] = useState<Set<string>>(new Set());

  const filtered = useMemo(() => {
    let out = rows ?? [];
    const s = q.trim().toLowerCase();
    if (s && search) out = out.filter((r) => search(r).toLowerCase().includes(s));
    const col = sort && columns.find((c) => c.key === sort.key);
    if (col?.sort) {
      const f = col.sort;
      out = [...out].sort((a, b) => {
        const x = f(a);
        const y = f(b);
        const c = typeof x === "number" && typeof y === "number" ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true });
        return sort!.dir === "asc" ? c : -c;
      });
    }
    return out;
  }, [rows, q, sort, columns, search]);

  const pages = Math.max(1, Math.ceil(filtered.length / size));
  useEffect(() => {
    if (page >= pages) setPage(pages - 1);
  }, [page, pages]);
  // Drop selections that no longer exist (deleted rows).
  useEffect(() => {
    if (!rows) return;
    const keys = new Set(rows.map(rowKey));
    setPicked((p) => {
      const n = new Set([...p].filter((k) => keys.has(k)));
      return n.size === p.size ? p : n;
    });
  }, [rows, rowKey]);

  const visible = filtered.slice(page * size, page * size + size);
  const selectedRows = (rows ?? []).filter((r) => picked.has(rowKey(r)));
  const allOnPage = visible.length > 0 && visible.every((r) => picked.has(rowKey(r)));

  const toggleAll = () => {
    const n = new Set(picked);
    for (const r of visible) allOnPage ? n.delete(rowKey(r)) : n.add(rowKey(r));
    setPicked(n);
  };
  const toggle = (k: string) => {
    const n = new Set(picked);
    n.has(k) ? n.delete(k) : n.add(k);
    setPicked(n);
  };
  const sortBy = (key: string) =>
    setSort((s) => (s?.key === key ? (s.dir === "asc" ? { key, dir: "desc" } : undefined) : { key, dir: "asc" }));

  const showToolbar = search || toolbar || (bulk && selectedRows.length > 0);

  return (
    <div className="min-w-0 rounded-lg border border-line bg-surface shadow-card">
      {showToolbar && (
        <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2.5">
          {bulk && selectedRows.length > 0 ? (
            <div className="flex flex-wrap items-center gap-2" role="toolbar" aria-label="Bulk actions">
              <span className="text-[12.5px] font-medium text-ink">{selectedRows.length} selected</span>
              {bulk.map((b) => (
                <Button key={b.label} size="sm" icon={b.icon} variant={b.danger ? "danger" : "secondary"} onClick={() => b.onRun(selectedRows)}>
                  {b.label}
                </Button>
              ))}
              <Button size="sm" variant="ghost" onClick={() => setPicked(new Set())}>
                Clear
              </Button>
            </div>
          ) : (
            search && (
              <div className="relative w-full max-w-xs">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" aria-hidden />
                <Input
                  value={q}
                  onChange={(e) => {
                    setQ(e.target.value);
                    setPage(0);
                  }}
                  placeholder={searchPlaceholder}
                  aria-label={searchPlaceholder}
                  className="pl-8"
                />
              </div>
            )
          )}
          {toolbar && <div className="ml-auto flex flex-wrap items-center gap-2">{toolbar}</div>}
        </div>
      )}

      {loading ? (
        <Loading />
      ) : error ? (
        <div className="p-4">
          <ErrorNote error={error} />
        </div>
      ) : !rows?.length ? (
        <EmptyState {...empty} />
      ) : !filtered.length ? (
        <EmptyState icon={Search} title="No matches" description={`Nothing matches “${q}”.`} />
      ) : (
        <StackTable>
            <thead>
              <tr>
                {bulk && (
                  <th className="w-9 border-b border-line bg-surface2 px-3 py-2">
                    <Checkbox checked={allOnPage} onChange={toggleAll} aria-label="Select all on this page" />
                  </th>
                )}
                {columns.map((c) => (
                  <th
                    key={c.key}
                    scope="col"
                    aria-sort={sort?.key === c.key ? (sort.dir === "asc" ? "ascending" : "descending") : undefined}
                    className={clsx(
                      "whitespace-nowrap border-b border-line bg-surface2 px-4 py-2 text-[11.5px] font-semibold uppercase tracking-wide text-muted",
                      c.hideSm && "hidden sm:table-cell",
                      c.className,
                    )}
                  >
                    {c.sort ? (
                      <button onClick={() => sortBy(c.key)} className="inline-flex items-center gap-1 uppercase hover:text-ink">
                        {c.header}
                        {sort?.key === c.key && (sort.dir === "asc" ? <ArrowUp className="size-3" aria-hidden /> : <ArrowDown className="size-3" aria-hidden />)}
                      </button>
                    ) : (
                      c.header
                    )}
                  </th>
                ))}
                {actions && (
                  <th className="border-b border-line bg-surface2 px-4 py-2">
                    <span className="sr-only">Actions</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {visible.map((r) => {
                const k = rowKey(r);
                return (
                  <tr
                    key={k}
                    className={clsx(
                      "transition-colors hover:bg-surface2/60 [&:last-child>td]:border-b-0",
                      picked.has(k) && "bg-accent-soft/40",
                      rowClassName?.(r),
                    )}
                  >
                    {bulk && (
                      <td className="border-b border-line px-3 py-2.5">
                        <Checkbox checked={picked.has(k)} onChange={() => toggle(k)} aria-label="Select row" />
                      </td>
                    )}
                    {columns.map((c) => (
                      <td key={c.key} className={clsx("border-b border-line px-4 py-2.5 align-middle", c.hideSm && "hidden sm:table-cell", c.className)}>
                        {c.render(r)}
                      </td>
                    ))}
                    {actions && (
                      <td className="border-b border-line px-3 py-1.5">
                        <div className="flex items-center justify-end gap-0.5">{actions(r)}</div>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
        </StackTable>
      )}

      {filtered.length > 10 && (
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line px-3 py-2 text-[12.5px] text-muted">
          <span>
            {page * size + 1}–{Math.min(filtered.length, (page + 1) * size)} of {filtered.length}
          </span>
          <div className="flex items-center gap-2">
            <label className="flex items-center gap-1.5">
              <span>Rows</span>
              <Select
                value={size}
                onChange={(e) => {
                  setSize(Number(e.target.value));
                  setPage(0);
                }}
                className="h-7 w-16"
                aria-label="Rows per page"
              >
                {[10, 25, 50, 100].map((n) => (
                  <option key={n}>{n}</option>
                ))}
              </Select>
            </label>
            <Button size="sm" variant="ghost" icon={ChevronLeft} disabled={page === 0} onClick={() => setPage(page - 1)} aria-label="Previous page" />
            <span className="tabular-nums">
              {page + 1} / {pages}
            </span>
            <Button size="sm" variant="ghost" icon={ChevronRight} disabled={page >= pages - 1} onClick={() => setPage(page + 1)} aria-label="Next page" />
          </div>
        </div>
      )}
    </div>
  );
}
