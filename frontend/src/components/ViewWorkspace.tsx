import { useProject } from "../state/ProjectContext";
import { MaskEditor } from "./mask-editor/MaskEditor";
import { InventoryPanel } from "./InventoryPanel";
import { RenderControls } from "./RenderControls";
import { RenderHistory } from "./RenderHistory";
import { ResultView } from "./result/ResultView";
import type { View } from "../types";

export function ViewWorkspace({ view }: { view: View }) {
  // Which render is shown is in the URL, so a link to one opens it.
  const { selectedRenderId, selectRender } = useProject();
  const selectedRender = view.renders.find((r) => r.id === selectedRenderId) ?? null;

  return (
    <div className="view-workspace">
      <MaskEditor view={view} />
      <InventoryPanel view={view} />
      <RenderControls
        view={view}
        onRendered={(renders) => {
          if (renders[0]) selectRender(view.id, renders[0].id);
        }}
      />
      {selectedRender && (
        <ResultView
          view={view}
          render={selectedRender}
          onEdited={(edit) => selectRender(view.id, edit.id)}
          onUpscaled={(upscale) => selectRender(view.id, upscale.id)}
        />
      )}
      <RenderHistory
        view={view}
        selectedRenderId={selectedRenderId}
        onSelect={(render) => selectRender(view.id, render.id)}
      />
    </div>
  );
}
