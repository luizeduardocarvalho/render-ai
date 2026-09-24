import { UserButton } from "@clerk/clerk-react";
import "./App.css";
import { AssetLibrary } from "./components/AssetLibrary";
import { ProjectPicker } from "./components/ProjectPicker";
import { StylePanel } from "./components/StylePanel";
import { ViewsBar } from "./components/ViewsBar";
import { ViewWorkspace } from "./components/ViewWorkspace";
import { useProject } from "./state/ProjectContext";

function App() {
  const { project, selectedView, closeProject } = useProject();

  if (!project) {
    return <ProjectPicker />;
  }

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <div className="app-topbar-left">
          <div className="app-brand">render-ai</div>
          <div className="app-project-name">{project.name}</div>
          <button type="button" className="btn btn-ghost btn-sm app-switch-project" onClick={closeProject}>
            Switch project
          </button>
        </div>
        <UserButton afterSignOutUrl="/sign-in" />
      </header>

      <div className="app-layout">
        <aside className="app-sidebar">
          <StylePanel />
          <AssetLibrary />
        </aside>

        <main className="app-main">
          <ViewsBar />
          {selectedView ? (
            <ViewWorkspace view={selectedView} />
          ) : (
            <div className="empty-state app-main-empty">
              <strong>No view selected</strong>
              <span>Upload a SketchUp screenshot to create your first view.</span>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

export default App;
