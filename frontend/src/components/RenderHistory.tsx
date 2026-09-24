import { useTranslation } from "react-i18next";
import type { Render, View } from "../types";
import { useProject } from "../state/ProjectContext";

interface RenderHistoryProps {
  view: View;
  selectedRenderId: string | null;
  onSelect: (render: Render) => void;
}

export function RenderHistory({ view, selectedRenderId, onSelect }: RenderHistoryProps) {
  const { t, i18n } = useTranslation();
  const { project } = useProject();
  const renders = [...view.renders].reverse();

  if (renders.length === 0) {
    return (
      <section className="panel">
        <div className="panel-header">
          <div className="panel-title">{t("renderHistory.title")}</div>
        </div>
        <div className="panel-body">
          <div className="empty-state">
            <strong>{t("renderHistory.empty.title")}</strong>
            <span>{t("renderHistory.empty.body")}</span>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("renderHistory.title")}</div>
          <div className="panel-subtitle">{t("renderHistory.subtitle", { count: renders.length })}</div>
        </div>
      </div>
      <div className="panel-body render-history-body">
        <table className="render-history-table">
          <thead>
            <tr>
              <th>{t("renderHistory.columns.created")}</th>
              <th>{t("renderHistory.columns.model")}</th>
              <th>{t("renderHistory.columns.resolution")}</th>
              <th>{t("renderHistory.columns.regions")}</th>
              <th>{t("renderHistory.columns.anchor")}</th>
              <th>{t("renderHistory.columns.imageCall")}</th>
              <th>{t("renderHistory.columns.total")}</th>
              <th>{t("renderHistory.columns.cost")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {renders.map((r) => {
              const isAnchor = project?.styleAnchorRenderId === r.id;
              const selected = r.id === selectedRenderId;
              return (
                <tr
                  key={r.id}
                  className={selected ? "render-row-selected" : ""}
                  onClick={() => onSelect(r)}
                >
                  <td>{new Date(r.createdAt).toLocaleString(i18n.language)}</td>
                  <td>{r.model}</td>
                  <td>{r.resolution}</td>
                  <td>{r.regionCount}</td>
                  <td>{r.anchorUsed ? t("common.yes") : t("common.no")}</td>
                  <td>{(r.metrics.imageCallMs / 1000).toFixed(1)}s</td>
                  <td>{(r.metrics.totalMs / 1000).toFixed(1)}s</td>
                  <td>
                    {r.metrics.estimatedCostUsd !== undefined
                      ? `$${r.metrics.estimatedCostUsd.toFixed(3)}`
                      : t("renderHistory.notAvailable")}
                  </td>
                  <td>{isAnchor && <span className="badge badge-accent">{t("renderHistory.anchorBadge")}</span>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
