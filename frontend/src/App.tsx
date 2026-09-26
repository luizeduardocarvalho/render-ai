import { UserButton } from "@clerk/clerk-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import "./App.css";
import { AssetLibrary } from "./components/AssetLibrary";
import { BrandMark, BrandSymbol } from "./components/BrandMark";
import { CreditsChip } from "./components/CreditsChip";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { NotificationsBell } from "./components/NotificationsBell";
import { ProjectPicker } from "./components/ProjectPicker";
import { StylePanel } from "./components/StylePanel";
import { ThemeToggle } from "./components/ThemeToggle";
import { ViewsBar } from "./components/ViewsBar";
import { ViewWorkspace } from "./components/ViewWorkspace";
import { useProject } from "./state/ProjectContext";

function App() {
  const { t } = useTranslation();
  const { project, openingProjectId, selectedView, closeProject, me } = useProject();

  if (!project) {
    // A link to a project: hold on the loading state instead of flashing the
    // project list.
    if (openingProjectId) {
      return (
        <div className="picker-screen">
          <div className="picker-status">
            <span className="spinner" /> {t("picker.loading")}
          </div>
        </div>
      );
    }
    return <ProjectPicker />;
  }

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <div className="app-topbar-left">
          <BrandMark />
          <div className="app-project-name">{project.name}</div>
          <button type="button" className="btn btn-ghost btn-sm app-switch-project" onClick={closeProject}>
            {t("app.switchProject")}
          </button>
        </div>
        <div className="app-topbar-right">
          <NotificationsBell />
          <CreditsChip />
          {me?.isAdmin && (
            <Link to="/admin" className="btn btn-ghost btn-sm">
              {t("admin.link")}
            </Link>
          )}
          <LanguageSwitcher />
          <ThemeToggle />
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
              <BrandSymbol oneColor className="empty-state-mark" />
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
