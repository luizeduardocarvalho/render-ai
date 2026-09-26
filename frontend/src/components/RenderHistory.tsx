import { useTranslation } from "react-i18next";
import type { Render, View } from "../types";
import { useProject } from "../state/ProjectContext";
import { BrandSymbol } from "./BrandMark";

// One row of the history table: a render, and how deeply it is nested under
// the render it was edited or upscaled from.
interface HistoryRow {
  render: Render;
  depth: number;
}

// Renders newest first, each followed by the renders made from it (oldest
// first, nested one level deeper per generation), so an edit or an upscale sits
// right under its source.
function buildRows(renders: Render[]): HistoryRow[] {
  const ids = new Set(renders.map((r) => r.id));
  const edits = new Map<string, Render[]>();
  const roots: Render[] = [];
  for (const r of renders) {
    const parentId = r.sourceRenderId ?? r.upscaledFromRenderId;
    if (parentId && ids.has(parentId)) {
      edits.set(parentId, [...(edits.get(parentId) ?? []), r]);
    } else {
      roots.push(r);
    }
  }
  const rows: HistoryRow[] = [];
  const visit = (render: Render, depth: number) => {
    rows.push({ render, depth });
    for (const edit of edits.get(render.id) ?? []) visit(edit, depth + 1);
  };
  for (const root of roots.toReversed()) visit(root, 0);
  return rows;
}

interface RenderHistoryProps {
  view: View;
  selectedRenderId: string | null;
  onSelect: (render: Render) => void;
}

export function RenderHistory({ view, selectedRenderId, onSelect }: RenderHistoryProps) {
  const { t, i18n } = useTranslation();
  const { project } = useProject();
  const renders = view.renders;
  const rows = buildRows(renders);

  if (renders.length === 0) {
    return (
      <section className="panel">
        <div className="panel-header">
          <div className="panel-title">{t("renderHistory.title")}</div>
        </div>
        <div className="panel-body">
          <div className="empty-state">
            <BrandSymbol oneColor className="empty-state-mark" />
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
            {rows.map(({ render: r, depth }) => {
              const isAnchor = project?.styleAnchorRenderId === r.id;
              const selected = r.id === selectedRenderId;
              return (
                <tr
                  key={r.id}
                  className={selected ? "render-row-selected" : ""}
                  onClick={() => onSelect(r)}
                >
                  <td>
                    <span className="render-row-created" style={{ paddingLeft: `${depth * 20}px` }}>
                      {depth > 0 && (
                        <span className="render-row-arrow" aria-hidden="true">
                          ↳
                        </span>
                      )}
                      <span className="render-row-created-text">
                        <span>{new Date(r.createdAt).toLocaleString(i18n.language)}</span>
                        {r.upscaledFromRenderId && (
                          <span className="render-row-edit-note">{t("renderHistory.upscaleNote")}</span>
                        )}
                        {r.editInstructions && r.editInstructions.length > 0 && (
                          <span className="render-row-edit-note" title={r.editInstructions.join("\n")}>
                            {t("renderHistory.editNote", { changes: r.editInstructions.join(" · ") })}
                          </span>
                        )}
                      </span>
                    </span>
                  </td>
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
