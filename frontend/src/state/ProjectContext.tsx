import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import * as api from "../api";
import type {
  Asset,
  Mask,
  Me,
  Project,
  ProjectSummary,
  Render,
  RenderJob,
  RenderRequest,
  StyleSettings,
  View,
} from "../types";

const LAST_PROJECT_KEY = "render-ai:lastProjectId";

interface ProjectContextValue {
  project: Project | null;

  // The signed-in user's identity + credit balance (GET /api/me). Fetched
  // once on mount and refreshed after anything that can change the balance
  // (renders, 402s, admin grants elsewhere).
  me: Me | null;
  meLoading: boolean;
  refreshMe: () => Promise<void>;

  // Project selection (the picker).
  projects: ProjectSummary[] | null;
  projectsLoading: boolean;
  projectsError: string | null;
  refreshProjects: () => Promise<void>;
  selectProject: (id: string) => Promise<void>;
  closeProject: () => void;
  deleteProject: (id: string) => Promise<void>;

  // Per-user asset library, shown on the project list screen (no project
  // open). Independent of `project.assets`, which the in-editor sidebar
  // keeps using via the project-scoped routes - both talk to the same
  // underlying per-user store on the backend.
  libraryAssets: Asset[] | null;
  libraryAssetsLoading: boolean;
  libraryAssetsError: string | null;
  refreshLibraryAssets: () => Promise<void>;
  createLibraryAsset: (data: { name: string; description: string; color: string }) => Promise<Asset>;
  updateLibraryAsset: (
    aid: string,
    data: { name: string; description: string; color: string },
  ) => Promise<Asset>;
  deleteLibraryAsset: (aid: string) => Promise<void>;
  uploadLibraryAssetReference: (aid: string, file: File, opts?: api.UploadOptions) => Promise<Asset>;

  selectedViewId: string | null;
  selectedView: View | null;
  setSelectedViewId: (id: string | null) => void;

  createProject: (name: string) => Promise<void>;
  updateStyle: (style: StyleSettings) => Promise<void>;
  setAnchor: (renderId: string | null) => Promise<void>;

  createAsset: (data: { name: string; description: string; color: string }) => Promise<Asset>;
  updateAsset: (
    aid: string,
    data: { name: string; description: string; color: string },
  ) => Promise<Asset>;
  deleteAsset: (aid: string) => Promise<void>;
  uploadAssetReference: (aid: string, file: File, opts?: api.UploadOptions) => Promise<Asset>;

  createView: (name: string, file: File, opts?: api.UploadOptions) => Promise<View>;
  deleteView: (vid: string) => Promise<void>;
  updateInventory: (vid: string, inventory: string) => Promise<void>;
  generateInventory: (vid: string) => Promise<string>;

  createMask: (vid: string, assetId?: string) => Promise<Mask>;
  updateMask: (
    vid: string,
    mid: string,
    data: { assetId?: string | null; hidden?: boolean },
  ) => Promise<Mask>;
  uploadMaskBitmap: (vid: string, mid: string, blob: Blob) => Promise<Mask>;
  deleteMask: (vid: string, mid: string) => Promise<void>;

  renderView: (
    vid: string,
    req: RenderRequest,
    opts?: { onProgress?: (job: RenderJob) => void; signal?: AbortSignal },
  ) => Promise<Render[]>;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

function replaceView(project: Project, vid: string, updater: (v: View) => View): Project {
  return {
    ...project,
    views: project.views.map((v) => (v.id === vid ? updater(v) : v)),
  };
}

export function ProjectProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const [project, setProject] = useState<Project | null>(null);
  const [selectedViewId, setSelectedViewId] = useState<string | null>(null);

  const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
  const [projectsLoading, setProjectsLoading] = useState(true);
  const [projectsError, setProjectsError] = useState<string | null>(null);

  const [me, setMe] = useState<Me | null>(null);
  const [meLoading, setMeLoading] = useState(true);

  const [libraryAssets, setLibraryAssets] = useState<Asset[] | null>(null);
  const [libraryAssetsLoading, setLibraryAssetsLoading] = useState(false);
  const [libraryAssetsError, setLibraryAssetsError] = useState<string | null>(null);

  const requireProject = useCallback(() => {
    if (!project) throw new Error("No active project");
    return project;
  }, [project]);

  const refreshMeFn = useCallback(async () => {
    setMeLoading(true);
    try {
      setMe(await api.getMe());
    } catch {
      // Balance chip / admin link just stay hidden - not worth a banner for this.
    } finally {
      setMeLoading(false);
    }
  }, []);

  useEffect(() => {
    void refreshMeFn();
  }, [refreshMeFn]);

  const refreshLibraryAssetsFn = useCallback(async () => {
    setLibraryAssetsLoading(true);
    setLibraryAssetsError(null);
    try {
      setLibraryAssets(await api.listAssets());
    } catch (err) {
      setLibraryAssetsError(err instanceof Error ? err.message : t("library.errors.loadFailed"));
    } finally {
      setLibraryAssetsLoading(false);
    }
  }, [t]);

  const createLibraryAssetFn = useCallback(
    async (data: { name: string; description: string; color: string }) => {
      const asset = await api.createLibraryAsset(data);
      setLibraryAssets((prev) => (prev ? [asset, ...prev] : [asset]));
      return asset;
    },
    [],
  );

  const updateLibraryAssetFn = useCallback(
    async (aid: string, data: { name: string; description: string; color: string }) => {
      const asset = await api.updateLibraryAsset(aid, data);
      setLibraryAssets((prev) => (prev ? prev.map((a) => (a.id === aid ? asset : a)) : prev));
      return asset;
    },
    [],
  );

  const deleteLibraryAssetFn = useCallback(async (aid: string) => {
    await api.deleteLibraryAsset(aid);
    setLibraryAssets((prev) => (prev ? prev.filter((a) => a.id !== aid) : prev));
  }, []);

  const uploadLibraryAssetReferenceFn = useCallback(
    async (aid: string, file: File, opts?: api.UploadOptions) => {
      const asset = await api.uploadLibraryAssetReference(aid, file, opts);
      setLibraryAssets((prev) => (prev ? prev.map((a) => (a.id === aid ? asset : a)) : prev));
      return asset;
    },
    [],
  );

  const refreshProjectsFn = useCallback(async () => {
    setProjectsLoading(true);
    setProjectsError(null);
    try {
      setProjects(await api.listProjects());
    } catch (err) {
      setProjectsError(err instanceof Error ? err.message : t("picker.errors.loadFailed"));
    } finally {
      setProjectsLoading(false);
    }
  }, [t]);

  // Load the project list once on mount.
  useEffect(() => {
    void refreshProjectsFn();
  }, [refreshProjectsFn]);

  const selectProjectFn = useCallback(async (id: string) => {
    const p = await api.getProject(id);
    setProject(p);
    setSelectedViewId(p.views[0]?.id ?? null);
    try {
      localStorage.setItem(LAST_PROJECT_KEY, id);
    } catch {
      // ignore storage failures (private mode, quota) - selection still works.
    }
  }, []);

  // Auto-open the last project once the list has loaded, if it still exists.
  useEffect(() => {
    if (project || projects === null) return;
    let lastId: string | null = null;
    try {
      lastId = localStorage.getItem(LAST_PROJECT_KEY);
    } catch {
      lastId = null;
    }
    if (lastId && projects.some((p) => p.id === lastId)) {
      void selectProjectFn(lastId);
    }
  }, [project, projects, selectProjectFn]);

  const closeProjectFn = useCallback(() => {
    setProject(null);
    setSelectedViewId(null);
    try {
      localStorage.removeItem(LAST_PROJECT_KEY);
    } catch {
      // ignore
    }
    void refreshProjectsFn();
  }, [refreshProjectsFn]);

  const deleteProjectFn = useCallback(
    async (id: string) => {
      await api.deleteProject(id);
      setProjects((prev) => (prev ? prev.filter((p) => p.id !== id) : prev));
      if (project?.id === id) {
        setProject(null);
        setSelectedViewId(null);
      }
      let lastId: string | null = null;
      try {
        lastId = localStorage.getItem(LAST_PROJECT_KEY);
      } catch {
        lastId = null;
      }
      if (lastId === id) {
        try {
          localStorage.removeItem(LAST_PROJECT_KEY);
        } catch {
          // ignore
        }
      }
    },
    [project?.id],
  );

  const createProjectFn = useCallback(
    async (name: string) => {
      const p = await api.createProject(name);
      setProject(p);
      setSelectedViewId(p.views[0]?.id ?? null);
      try {
        localStorage.setItem(LAST_PROJECT_KEY, p.id);
      } catch {
        // ignore
      }
      void refreshProjectsFn();
    },
    [refreshProjectsFn],
  );

  const updateStyleFn = useCallback(
    async (style: StyleSettings) => {
      const p = requireProject();
      const updated = await api.updateStyle(p.id, style);
      setProject(updated);
    },
    [requireProject],
  );

  const setAnchorFn = useCallback(
    async (renderId: string | null) => {
      const p = requireProject();
      const updated = await api.setAnchor(p.id, renderId);
      setProject(updated);
    },
    [requireProject],
  );

  const createAssetFn = useCallback(
    async (data: { name: string; description: string; color: string }) => {
      const p = requireProject();
      const asset = await api.createAsset(p.id, data);
      setProject((prev) => (prev ? { ...prev, assets: [...prev.assets, asset] } : prev));
      return asset;
    },
    [requireProject],
  );

  const updateAssetFn = useCallback(
    async (aid: string, data: { name: string; description: string; color: string }) => {
      const p = requireProject();
      const asset = await api.updateAsset(p.id, aid, data);
      setProject((prev) =>
        prev
          ? { ...prev, assets: prev.assets.map((a) => (a.id === aid ? asset : a)) }
          : prev,
      );
      return asset;
    },
    [requireProject],
  );

  const deleteAssetFn = useCallback(
    async (aid: string) => {
      const p = requireProject();
      await api.deleteAsset(p.id, aid);
      setProject((prev) =>
        prev ? { ...prev, assets: prev.assets.filter((a) => a.id !== aid) } : prev,
      );
    },
    [requireProject],
  );

  const uploadAssetReferenceFn = useCallback(
    async (aid: string, file: File, opts?: api.UploadOptions) => {
      const p = requireProject();
      const asset = await api.uploadAssetReference(p.id, aid, file, opts);
      setProject((prev) =>
        prev
          ? { ...prev, assets: prev.assets.map((a) => (a.id === aid ? asset : a)) }
          : prev,
      );
      return asset;
    },
    [requireProject],
  );

  const createViewFn = useCallback(
    async (name: string, file: File, opts?: api.UploadOptions) => {
      const p = requireProject();
      const view = await api.createView(p.id, name, file, opts);
      setProject((prev) => (prev ? { ...prev, views: [...prev.views, view] } : prev));
      setSelectedViewId(view.id);
      return view;
    },
    [requireProject],
  );

  const deleteViewFn = useCallback(
    async (vid: string) => {
      const p = requireProject();
      await api.deleteView(p.id, vid);
      setProject((prev) =>
        prev ? { ...prev, views: prev.views.filter((v) => v.id !== vid) } : prev,
      );
      setSelectedViewId((prev) => (prev === vid ? null : prev));
    },
    [requireProject],
  );

  const updateInventoryFn = useCallback(
    async (vid: string, inventory: string) => {
      const p = requireProject();
      const view = await api.updateInventory(p.id, vid, inventory);
      setProject((prev) => (prev ? replaceView(prev, vid, () => view) : prev));
    },
    [requireProject],
  );

  const generateInventoryFn = useCallback(
    async (vid: string) => {
      const p = requireProject();
      const { inventory } = await api.generateInventory(p.id, vid);
      setProject((prev) =>
        prev ? replaceView(prev, vid, (v) => ({ ...v, inventory })) : prev,
      );
      return inventory;
    },
    [requireProject],
  );

  const createMaskFn = useCallback(
    async (vid: string, assetId?: string) => {
      const p = requireProject();
      const mask = await api.createMask(p.id, vid, assetId);
      setProject((prev) =>
        prev ? replaceView(prev, vid, (v) => ({ ...v, masks: [...v.masks, mask] })) : prev,
      );
      return mask;
    },
    [requireProject],
  );

  const updateMaskFn = useCallback(
    async (vid: string, mid: string, data: { assetId?: string | null; hidden?: boolean }) => {
      const p = requireProject();
      const mask = await api.updateMask(p.id, vid, mid, data);
      setProject((prev) =>
        prev
          ? replaceView(prev, vid, (v) => ({
              ...v,
              masks: v.masks.map((m) => (m.id === mid ? mask : m)),
            }))
          : prev,
      );
      return mask;
    },
    [requireProject],
  );

  const uploadMaskBitmapFn = useCallback(
    async (vid: string, mid: string, blob: Blob) => {
      const p = requireProject();
      const mask = await api.uploadMaskBitmap(p.id, vid, mid, blob);
      setProject((prev) =>
        prev
          ? replaceView(prev, vid, (v) => ({
              ...v,
              masks: v.masks.map((m) => (m.id === mid ? mask : m)),
            }))
          : prev,
      );
      return mask;
    },
    [requireProject],
  );

  const deleteMaskFn = useCallback(
    async (vid: string, mid: string) => {
      const p = requireProject();
      await api.deleteMask(p.id, vid, mid);
      setProject((prev) =>
        prev
          ? replaceView(prev, vid, (v) => ({ ...v, masks: v.masks.filter((m) => m.id !== mid) }))
          : prev,
      );
    },
    [requireProject],
  );

  const renderViewFn = useCallback(
    async (
      vid: string,
      req: RenderRequest,
      opts?: { onProgress?: (job: RenderJob) => void; signal?: AbortSignal },
    ) => {
      const p = requireProject();
      const renders = await api.renderView(p.id, vid, req, opts);
      setProject((prev) =>
        prev
          ? replaceView(prev, vid, (v) => ({ ...v, renders: [...v.renders, ...renders] }))
          : prev,
      );
      return renders;
    },
    [requireProject],
  );

  const selectedView = useMemo(
    () => project?.views.find((v) => v.id === selectedViewId) ?? null,
    [project, selectedViewId],
  );

  const value: ProjectContextValue = {
    project,
    me,
    meLoading,
    refreshMe: refreshMeFn,
    projects,
    projectsLoading,
    projectsError,
    refreshProjects: refreshProjectsFn,
    selectProject: selectProjectFn,
    closeProject: closeProjectFn,
    deleteProject: deleteProjectFn,
    libraryAssets,
    libraryAssetsLoading,
    libraryAssetsError,
    refreshLibraryAssets: refreshLibraryAssetsFn,
    createLibraryAsset: createLibraryAssetFn,
    updateLibraryAsset: updateLibraryAssetFn,
    deleteLibraryAsset: deleteLibraryAssetFn,
    uploadLibraryAssetReference: uploadLibraryAssetReferenceFn,
    selectedViewId,
    selectedView,
    setSelectedViewId,
    createProject: createProjectFn,
    updateStyle: updateStyleFn,
    setAnchor: setAnchorFn,
    createAsset: createAssetFn,
    updateAsset: updateAssetFn,
    deleteAsset: deleteAssetFn,
    uploadAssetReference: uploadAssetReferenceFn,
    createView: createViewFn,
    deleteView: deleteViewFn,
    updateInventory: updateInventoryFn,
    generateInventory: generateInventoryFn,
    createMask: createMaskFn,
    updateMask: updateMaskFn,
    uploadMaskBitmap: uploadMaskBitmapFn,
    deleteMask: deleteMaskFn,
    renderView: renderViewFn,
  };

  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

export function useProject(): ProjectContextValue {
  const ctx = useContext(ProjectContext);
  if (!ctx) throw new Error("useProject must be used within ProjectProvider");
  return ctx;
}
