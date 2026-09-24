import { useState } from "react";
import { MaskEditor } from "./mask-editor/MaskEditor";
import { InventoryPanel } from "./InventoryPanel";
import { RenderControls } from "./RenderControls";
import { RenderHistory } from "./RenderHistory";
import { ResultView } from "./result/ResultView";
import type { View } from "../types";

export function ViewWorkspace({ view }: { view: View }) {
  const [selectedRenderId, setSelectedRenderId] = useState<string | null>(
    view.renders.at(-1)?.id ?? null,
  );

  // Reset the visible result whenever the user switches views (adjust state
  // during render rather than in an effect, so there's no stale-view flash).
  const [lastViewId, setLastViewId] = useState(view.id);
  if (lastViewId !== view.id) {
    setLastViewId(view.id);
    setSelectedRenderId(view.renders.at(-1)?.id ?? null);
  }

  const selectedRender = view.renders.find((r) => r.id === selectedRenderId) ?? null;

  return (
    <div className="view-workspace">
      <MaskEditor view={view} />
      <InventoryPanel view={view} />
      <RenderControls
        view={view}
        onRendered={(renders) => setSelectedRenderId(renders[0]?.id ?? null)}
      />
      {selectedRender && (
        <ResultView
          view={view}
          render={selectedRender}
          onEdited={(edit) => setSelectedRenderId(edit.id)}
        />
      )}
      <RenderHistory
        view={view}
        selectedRenderId={selectedRenderId}
        onSelect={(render) => setSelectedRenderId(render.id)}
      />
    </div>
  );
}
