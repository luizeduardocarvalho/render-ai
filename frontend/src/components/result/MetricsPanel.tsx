import { useEffect, useState } from "react";
import { getPricing } from "../../api";
import type { Render } from "../../types";

const brlFormatter = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });

function fmtMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function fmtCost(usd: number | undefined, usdToBrl: number | undefined): string {
  if (usd === undefined) return "n/a";
  const usdPart = `$${usd.toFixed(usd < 0.01 ? 4 : 2)}`;
  if (usdToBrl === undefined) return usdPart;
  return `${usdPart} (≈ ${brlFormatter.format(usd * usdToBrl)})`;
}

export function MetricsPanel({ render }: { render: Render }) {
  const m = render.metrics;
  const preservation = render.preservation;
  const [usdToBrl, setUsdToBrl] = useState<number | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    getPricing()
      .then((p) => {
        if (!cancelled) setUsdToBrl(p.usdToBrl);
      })
      .catch(() => {
        // BRL conversion is a nice-to-have - fall back to USD-only below.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="metrics-panel">
      <dl className="metrics-grid">
        <Metric label="Model" value={m.model} />
        <Metric label="Resolution" value={m.resolution} />
        <Metric label="Regions" value={String(m.regionCount)} />
        <Metric label="Anchor used" value={m.anchorUsed ? "Yes" : "No"} />
        <Metric label="Seed" value="n/a (not supported)" />
        <Metric label="Image call latency" value={fmtMs(m.imageCallMs)} />
        <Metric label="Total latency" value={fmtMs(m.totalMs)} />
        <Metric label="Prompt tokens" value={m.promptTokens !== undefined ? String(m.promptTokens) : "n/a"} />
        <Metric label="Output tokens" value={m.outputTokens !== undefined ? String(m.outputTokens) : "n/a"} />
        <Metric label="Estimated cost" value={fmtCost(m.estimatedCostUsd, usdToBrl)} />
      </dl>

      {preservation && (
        <div className="preservation-report">
          <div className="preservation-header">
            <span className="field-label">Preservation check</span>
            <span className={`badge ${preservation.edgeFlag ? "badge-warning" : "badge-success"}`}>
              Edge score {preservation.edgeScore.toFixed(2)}
              {preservation.edgeFlag ? " - below threshold" : ""}
            </span>
          </div>
          <div className="preservation-lists">
            <PreservationList label="Removed" items={preservation.inventory.removed} tone="danger" />
            <PreservationList label="Added" items={preservation.inventory.added} tone="accent" />
            <PreservationList label="Moved" items={preservation.inventory.moved} tone="warning" />
          </div>
          {preservation.inventory.raw && (
            <details className="preservation-raw">
              <summary>Raw model notes</summary>
              <pre>{preservation.inventory.raw}</pre>
            </details>
          )}
        </div>
      )}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric-item">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function PreservationList({
  label,
  items,
  tone,
}: {
  label: string;
  items: string[] | null | undefined;
  tone: "danger" | "accent" | "warning";
}) {
  // The backend may serialize an empty diff list as null (Go nil slice), so
  // normalize before reading .length.
  const list = items ?? [];
  return (
    <div className="preservation-list">
      <span className={`badge badge-${tone}`}>
        {label} ({list.length})
      </span>
      {list.length > 0 ? (
        <ul>
          {list.map((item, i) => (
            <li key={i}>{item}</li>
          ))}
        </ul>
      ) : (
        <p className="field-hint">None</p>
      )}
    </div>
  );
}
