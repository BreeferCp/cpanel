import { useEffect, useMemo, useRef, useState } from "react";

export interface Series {
  name: string;
  /** CSS colour, normally var(--series-N); fixed per entity, never by rank. */
  color: string;
  values: number[];
}

interface Props {
  /** Unix seconds, one per value. */
  times: number[];
  series: Series[];
  format: (v: number) => string;
  /** Fixed y maximum (e.g. 100 for percentages); auto when omitted. */
  max?: number;
  height?: number;
  /** Accessible summary of what the chart shows. */
  label: string;
}

const PAD = { top: 8, right: 12, bottom: 22, left: 60 };

function niceMax(v: number): number {
  if (v <= 0) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p;
  return 10 * p;
}

const timeFmt = (t: number, spanH: number) =>
  new Date(t * 1000).toLocaleString(undefined, spanH > 30 ? { month: "short", day: "numeric" } : { hour: "2-digit", minute: "2-digit" });

/**
 * Line/area chart drawn as SVG: one y-axis, 2px lines, recessive grid, a
 * crosshair with a tooltip on hover, a legend plus direct end labels when
 * there are two or more series, and a table view for accessibility.
 */
export function TimeChart({ times, series, format, max, height = 180, label }: Props) {
  const wrap = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(600);
  const [hover, setHover] = useState<number | null>(null);
  const [table, setTable] = useState(false);

  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => e && setWidth(Math.max(200, Math.floor(e.contentRect.width))));
    ro.observe(el);
    return () => ro.disconnect();
  }, [times.length < 2]);

  const n = times.length;
  const yMax = useMemo(() => max ?? niceMax(Math.max(0, ...series.flatMap((s) => s.values)) * 1.1), [max, series]);
  const multi = series.length > 1;
  const padR = multi ? 40 : PAD.right; // room for direct end labels
  const iw = width - PAD.left - padR;
  const ih = height - PAD.top - PAD.bottom;
  const x = (i: number) => PAD.left + (n <= 1 ? 0 : (i / (n - 1)) * iw);
  const y = (v: number) => PAD.top + ih - (Math.min(v, yMax) / yMax) * ih;
  const spanH = n > 1 ? (times[n - 1]! - times[0]!) / 3600 : 0;

  if (n < 2) {
    // Keep the measured wrapper mounted so the width is known once data arrives.
    return (
      <div ref={wrap} className="flex items-center justify-center text-[12.5px] text-muted" style={{ height }}>
        Collecting data…
      </div>
    );
  }

  const path = (vals: number[]) => vals.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join("");
  const ticksY = [0, 0.5, 1].map((f) => f * yMax);
  const ticksX = [0, Math.floor((n - 1) / 3), Math.floor(((n - 1) * 2) / 3), n - 1];

  const onMove = (e: React.PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    const px = e.clientX - r.left - PAD.left;
    setHover(Math.max(0, Math.min(n - 1, Math.round((px / iw) * (n - 1)))));
  };

  return (
    <div>
      {multi && (
        <div className="mb-2 flex flex-wrap gap-4 text-[12px] text-muted" aria-hidden>
          {series.map((s) => (
            <span key={s.name} className="inline-flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-3 rounded-full" style={{ background: s.color }} />
              {s.name}
            </span>
          ))}
        </div>
      )}
      <div ref={wrap} className="relative min-w-0">
        <svg
          width={width}
          height={height}
          role="img"
          aria-label={label}
          onPointerMove={onMove}
          onPointerLeave={() => setHover(null)}
          className="block touch-none select-none"
        >
          {ticksY.map((v) => (
            <g key={v}>
              <line x1={PAD.left} x2={width - padR} y1={y(v)} y2={y(v)} stroke="var(--grid)" strokeWidth={1} />
              <text x={PAD.left - 8} y={y(v)} dy="0.32em" textAnchor="end" fontSize={11} fill="var(--subtle)">
                {format(v)}
              </text>
            </g>
          ))}
          {ticksX.map((i, k) => (
            <text
              key={k}
              x={x(i)}
              y={height - 6}
              textAnchor={k === 0 ? "start" : k === ticksX.length - 1 ? "end" : "middle"}
              fontSize={11}
              fill="var(--subtle)"
            >
              {timeFmt(times[i]!, spanH)}
            </text>
          ))}
          {series.map((s) => (
            <g key={s.name}>
              {!multi && (
                <path
                  d={`${path(s.values)}L${x(n - 1)},${y(0)}L${x(0)},${y(0)}Z`}
                  fill={s.color}
                  opacity={0.08}
                />
              )}
              <path d={path(s.values)} fill="none" stroke={s.color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
              {multi && (
                <text x={x(n - 1) + 6} y={y(s.values[n - 1] ?? 0)} dy="0.32em" fontSize={11} fill="var(--muted)">
                  {s.name}
                </text>
              )}
            </g>
          ))}
          {hover !== null && (
            <g>
              <line x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + ih} stroke="var(--line-strong)" strokeWidth={1} />
              {series.map((s) => (
                <circle key={s.name} cx={x(hover)} cy={y(s.values[hover] ?? 0)} r={4} fill={s.color} stroke="var(--surface)" strokeWidth={2} />
              ))}
            </g>
          )}
        </svg>
        {hover !== null && (
          <div
            className="pointer-events-none absolute top-1 z-10 min-w-32 rounded-md border border-line bg-surface px-2.5 py-1.5 text-[12px] shadow-pop"
            style={x(hover) > width / 2 ? { right: width - x(hover) + 10 } : { left: x(hover) + 10 }}
          >
            <div className="mb-0.5 text-muted">{new Date(times[hover]! * 1000).toLocaleString()}</div>
            {series.map((s) => (
              <div key={s.name} className="flex items-center justify-between gap-3">
                <span className="inline-flex items-center gap-1.5 text-muted">
                  <span className="inline-block size-2 rounded-full" style={{ background: s.color }} />
                  {s.name}
                </span>
                <span className="font-medium tabular-nums text-ink">{format(s.values[hover] ?? 0)}</span>
              </div>
            ))}
          </div>
        )}
      </div>
      <button className="mt-1 text-[11.5px] text-subtle hover:text-ink" onClick={() => setTable(!table)}>
        {table ? "Hide table" : "View as table"}
      </button>
      {table && (
        <div className="mt-2 max-h-48 overflow-y-auto rounded border border-line">
          <table className="w-full text-[12px]">
            <thead className="sticky top-0 bg-surface2 text-muted">
              <tr>
                <th className="px-2 py-1 text-left font-medium">Time</th>
                {series.map((s) => (
                  <th key={s.name} className="px-2 py-1 text-right font-medium">
                    {s.name}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {times
                .map((t, i) => ({ t, i }))
                .reverse()
                .map(({ t, i }) => (
                  <tr key={t} className="border-t border-line">
                    <td className="px-2 py-1 text-muted">{new Date(t * 1000).toLocaleString()}</td>
                    {series.map((s) => (
                      <td key={s.name} className="px-2 py-1 text-right tabular-nums">
                        {format(s.values[i] ?? 0)}
                      </td>
                    ))}
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** Small inline trend line for stat tiles (decorative; the tile carries the number). */
export function Sparkline({ values, color = "var(--series-1)" }: { values: number[]; color?: string }) {
  if (values.length < 2) return null;
  const w = 96,
    h = 28;
  const mx = Math.max(...values) || 1;
  const d = values.map((v, i) => `${i ? "L" : "M"}${((i / (values.length - 1)) * w).toFixed(1)},${(h - 2 - (v / mx) * (h - 4)).toFixed(1)}`).join("");
  return (
    <svg width={w} height={h} aria-hidden className="shrink-0">
      <path d={d} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
