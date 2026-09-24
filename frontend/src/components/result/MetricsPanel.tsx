import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getPricing } from "../../api";
import type { Render } from "../../types";

function fmtMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function MetricsPanel({ render }: { render: Render }) {
  const { t, i18n } = useTranslation();
  const m = render.metrics;
  const preservation = render.preservation;
  const [usdToBrl, setUsdToBrl] = useState<number | undefined>(undefined);

  const usdFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.language, { style: "currency", currency: "USD", maximumFractionDigits: 4 }),
    [i18n.language],
  );
  const brlFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.language, { style: "currency", currency: "BRL" }),
    [i18n.language],
  );

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

  function fmtCost(usd: number | undefined): string {
    if (usd === undefined) return t("metricsPanel.notAvailable");
    const usdPart = usdFormatter.format(usd);
    if (usdToBrl === undefined) return usdPart;
    return `${usdPart} (≈ ${brlFormatter.format(usd * usdToBrl)})`;
  }

  return (
    <div className="metrics-panel">
      <dl className="metrics-grid">
        <Metric label={t("metricsPanel.model")} value={m.model} />
        <Metric label={t("metricsPanel.resolution")} value={m.resolution} />
        <Metric label={t("metricsPanel.regions")} value={String(m.regionCount)} />
        <Metric label={t("metricsPanel.anchorUsed")} value={m.anchorUsed ? t("common.yes") : t("common.no")} />
        <Metric label={t("metricsPanel.seed")} value={t("metricsPanel.seedValue")} />
        <Metric label={t("metricsPanel.imageCallLatency")} value={fmtMs(m.imageCallMs)} />
        <Metric label={t("metricsPanel.totalLatency")} value={fmtMs(m.totalMs)} />
        <Metric
          label={t("metricsPanel.promptTokens")}
          value={m.promptTokens !== undefined ? String(m.promptTokens) : t("metricsPanel.notAvailable")}
        />
        <Metric
          label={t("metricsPanel.outputTokens")}
          value={m.outputTokens !== undefined ? String(m.outputTokens) : t("metricsPanel.notAvailable")}
        />
        <Metric label={t("metricsPanel.estimatedCost")} value={fmtCost(m.estimatedCostUsd)} />
      </dl>

      {preservation && (
        <div className="preservation-report">
          <div className="preservation-header">
            <span className="field-label">{t("metricsPanel.preservationCheck")}</span>
            <span className={`badge ${preservation.edgeFlag ? "badge-warning" : "badge-success"}`}>
              {t("metricsPanel.edgeScore", { score: preservation.edgeScore.toFixed(2) })}
              {preservation.edgeFlag ? t("metricsPanel.belowThreshold") : ""}
            </span>
          </div>
          <div className="preservation-lists">
            <PreservationList label={t("metricsPanel.removed")} items={preservation.inventory.removed} tone="danger" />
            <PreservationList label={t("metricsPanel.added")} items={preservation.inventory.added} tone="accent" />
            <PreservationList label={t("metricsPanel.moved")} items={preservation.inventory.moved} tone="warning" />
          </div>
          {preservation.inventory.raw && (
            <details className="preservation-raw">
              <summary>{t("metricsPanel.rawNotes")}</summary>
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
  const { t } = useTranslation();
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
        <p className="field-hint">{t("metricsPanel.none")}</p>
      )}
    </div>
  );
}
