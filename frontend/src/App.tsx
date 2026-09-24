import { UserButton } from "@clerk/clerk-react";
import { useTranslation } from "react-i18next";
import "./App.css";
import { AssetLibrary } from "./components/AssetLibrary";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { ProjectPicker } from "./components/ProjectPicker";
import { StylePanel } from "./components/StylePanel";
import { ViewsBar } from "./components/ViewsBar";
import { ViewWorkspace } from "./components/ViewWorkspace";
import { useProject } from "./state/ProjectContext";

function App() {
  const { t } = useTranslation();
  const { project, selectedView, closeProject } = useProject();

  if (!project) {
    return <ProjectPicker />;
  }

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <div className="app-topbar-left">
          <div className="app-brand">{t("app.brand")}</div>
          <div className="app-project-name">{project.name}</div>
          <button type="button" className="btn btn-ghost btn-sm app-switch-project" onClick={closeProject}>
            {t("app.switchProject")}
          </button>
        </div>
        <div className="app-topbar-right">
          <LanguageSwitcher />
          <UserButton afterSignOutUrl="/sign-in" />
        </div>
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
              <strong>{t("app.noViewSelected.title")}</strong>
              <span>{t("app.noViewSelected.body")}</span>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

export default App;
