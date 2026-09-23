import type { Render, View } from "../types";
import { useProject } from "../state/ProjectContext";

interface RenderHistoryProps {
  view: View;
  selectedRenderId: string | null;
  onSelect: (render: Render) => void;
}

export function RenderHistory({ view, selectedRenderId, onSelect }: RenderHistoryProps) {
  const { project } = useProject();
  const renders = [...view.renders].reverse();

  if (renders.length === 0) {
    return (
      <section className="panel">
        <div className="panel-header">
          <div className="panel-title">Render history</div>
        </div>
        <div className="panel-body">
          <div className="empty-state">
            <strong>No renders yet</strong>
            <span>Generate a render above to see it here.</span>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">Render history</div>
          <div className="panel-subtitle">
            {renders.length} render{renders.length === 1 ? "" : "s"} for this view
          </div>
        </div>
      </div>
      <div className="panel-body render-history-body">
        <table className="render-history-table">
          <thead>
            <tr>
              <th>Created</th>
              <th>Model</th>
              <th>Res</th>
              <th>Regions</th>
              <th>Anchor</th>
              <th>Image call</th>
              <th>Total</th>
              <th>Cost</th>
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
                  <td>{new Date(r.createdAt).toLocaleString()}</td>
                  <td>{r.model}</td>
                  <td>{r.resolution}</td>
                  <td>{r.regionCount}</td>
                  <td>{r.anchorUsed ? "Yes" : "No"}</td>
                  <td>{(r.metrics.imageCallMs / 1000).toFixed(1)}s</td>
                  <td>{(r.metrics.totalMs / 1000).toFixed(1)}s</td>
                  <td>{r.metrics.estimatedCostUsd !== undefined ? `$${r.metrics.estimatedCostUsd.toFixed(3)}` : "n/a"}</td>
                  <td>{isAnchor && <span className="badge badge-accent">Anchor</span>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
