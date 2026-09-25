import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import * as api from "../api";
import { announceJobsChanged } from "../lib/jobEvents";
import { appPath, parseAppPath } from "../lib/routes";
import type {
  Asset,
  EditRegionRequest,
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
  // The open project: the one the URL names, once it has loaded. Null on the
  // project list, and while a project named by the URL is still loading.
  project: Project | null;
  // The id of a project the URL names that has not loaded yet.
  openingProjectId: string | null;
  // Why the last project the URL named could not be opened (it was sent back
  // to the project list). Cleared when a project opens.
  openError: string | null;
  // Reloads the open project from the server, e.g. for a render made while
  // its screen was not open.
  refreshProject: () => Promise<void>;

  // The signed-in user's identity + credit balance (GET /api/me). Fetched
  // once on mount and refreshed after anything that can change the balance
  // (renders, 402s, admin grants elsewhere).
  me: Me | null;
  meLoading: boolean;
  refreshMe: () => Promise<void>;
  // Shifts the displayed balance by delta credits without a round trip, so
  // the header reacts the moment a render is started (or a variation fails
  // and is refunded). The next refreshMe() replaces it with the server's
  // balance, which stays the source of truth.
  adjustDisplayedCredits: (delta: number) => void;

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

  // The view and render shown, from the URL (the first view, and the latest
  // render, when it names none).
  selectedViewId: string | null;
  selectedView: View | null;
  selectView: (vid: string) => void;
  selectedRenderId: string | null;
  // Shows a render of a view, unless the user has moved to another view since
  // (a render that finishes in the background must not pull them back).
  selectRender: (vid: string, rid: string) => void;

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
  editRender: (
    vid: string,
    rid: string,
    regions: EditRegionRequest[],
    opts?: { onProgress?: (job: RenderJob) => void; signal?: AbortSignal },
  ) => Promise<Render[]>;
  upscaleRender: (
    vid: string,
    rid: string,
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

type JobOptions = { onProgress?: (job: RenderJob) => void; signal?: AbortSignal };

// Tells the notification bell when a job starts and when it ends, so its list
// is current at once instead of at its next poll.
function announcingJob(opts?: JobOptions): JobOptions {
  let started = false;
  return {
    ...opts,
    onProgress: (job) => {
      opts?.onProgress?.(job);
      const ended = job.status === "done" || job.status === "failed";
      if (!started || ended) announceJobsChanged();
      started = true;
    },
  };
}

// Adds finished renders to a view, skipping any it already has: a project
// reloaded from the server while a render was polling can already hold them.
function withRenders(view: View, renders: Render[]): View {
  const known = new Set(view.renders.map((r) => r.id));
  return { ...view, renders: [...view.renders, ...renders.filter((r) => !known.has(r.id))] };
}

export function ProjectProvider({ children }: { children: ReactNode }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const route = useMemo(() => parseAppPath(location.pathname), [location.pathname]);

  // The last project loaded from the server. It is the open project only
  // while the URL names it (see `project` below).
  const [loadedProject, setProject] = useState<Project | null>(null);
  const [openError, setOpenError] = useState<string | null>(null);
  // The render a URL named that was not in the loaded project and has been
  // looked for on the server since: its `pid/rid` key.
  const [settledRid, setSettledRid] = useState<string | null>(null);
  const fetchingRid = useRef<string | null>(null);
  const project = loadedProject && loadedProject.id === route.pid ? loadedProject : null;

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

  const adjustDisplayedCreditsFn = useCallback((delta: number) => {
    setMe((prev) => (prev ? { ...prev, credits: Math.max(0, prev.credits + delta) } : prev));
  }, []);

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

  // Fetched once on mount. (Not through refreshMeFn: that sets its loading
  // state synchronously, which an effect should not.)
  useEffect(() => {
    let cancelled = false;
    api
      .getMe()
      .then((m) => {
        if (!cancelled) setMe(m);
      })
      .catch(() => {
        // Balance chip / admin link just stay hidden - not worth a banner for this.
      })
      .finally(() => {
        if (!cancelled) setMeLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

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

  // Load the project list once on mount (it starts out loading).
  useEffect(() => {
    let cancelled = false;
    api
      .listProjects()
      .then((list) => {
        if (!cancelled) setProjects(list);
      })
      .catch((err) => {
        if (!cancelled) {
          setProjectsError(err instanceof Error ? err.message : t("picker.errors.loadFailed"));
        }
      })
      .finally(() => {
        if (!cancelled) setProjectsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [t]);

  // Reloads the list without the loading state, so a list already on screen
  // stays put while it updates.
  const reloadProjectsQuietly = useCallback(() => {
    api
      .listProjects()
      .then(setProjects)
      .catch(() => {
        // The list on screen is only a little stale - not worth a banner.
      });
  }, []);

  // The project the URL names is loaded when it is not the one already held.
  useEffect(() => {
    if (!route.pid || loadedProject?.id === route.pid) return;
    let cancelled = false;
    api
      .getProject(route.pid)
      .then((p) => {
        if (cancelled) return;
        setOpenError(null);
        setProject(p);
      })
      .catch((err) => {
        if (cancelled) return;
        setOpenError(err instanceof api.ApiError ? err.message : t("picker.errors.openFailed"));
        navigate("/", { replace: true });
      });
    return () => {
      cancelled = true;
    };
  }, [route.pid, loadedProject?.id, navigate, t]);

  const refreshProjectFn = useCallback(async () => {
    const pid = route.pid;
    if (!pid) return;
    try {
      const p = await api.getProject(pid);
      // Only if it is still the project on screen.
      setProject((cur) => (cur && cur.id === p.id ? p : cur));
    } catch {
      // The project on screen is only a little stale - keep it.
    }
  }, [route.pid]);

  const selectProjectFn = useCallback(
    async (id: string) => {
      const p = await api.getProject(id);
      setOpenError(null);
      setProject(p);
      navigate(appPath(p.id, p.views[0]?.id));
    },
    [navigate],
  );

  // Remember the open project, and open it again next time the app starts.
  const openId = project?.id ?? null;
  useEffect(() => {
    if (!openId) return;
    try {
      localStorage.setItem(LAST_PROJECT_KEY, openId);
    } catch {
      // ignore storage failures (private mode, quota) - the project still opens.
    }
  }, [openId]);

  // Once the list has loaded, on the app's first screen only, open the last
  // project if it still exists. Later visits to the list (the back button)
  // are the user's choice and are not bounced forward.
  const autoOpened = useRef(false);
  useEffect(() => {
    if (autoOpened.current || projects === null) return;
    autoOpened.current = true;
    if (route.pid) return;
    let lastId: string | null = null;
    try {
      lastId = localStorage.getItem(LAST_PROJECT_KEY);
    } catch {
      lastId = null;
    }
    if (lastId && projects.some((p) => p.id === lastId)) {
      navigate(appPath(lastId), { replace: true });
    }
  }, [projects, route.pid, navigate]);

  // Back on the project list after a project: its card (renders, thumbnail)
  // may have changed.
  const openPid = useRef<string | null>(null);
  useEffect(() => {
    if (openPid.current && !route.pid) reloadProjectsQuietly();
    openPid.current = route.pid;
  }, [route.pid, reloadProjectsQuietly]);

  const closeProjectFn = useCallback(() => {
    try {
      localStorage.removeItem(LAST_PROJECT_KEY);
    } catch {
      // ignore
    }
    navigate("/");
  }, [navigate]);

  const deleteProjectFn = useCallback(
    async (id: string) => {
      await api.deleteProject(id);
      setProjects((prev) => (prev ? prev.filter((p) => p.id !== id) : prev));
      if (route.pid === id) navigate("/", { replace: true });
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
    [route.pid, navigate],
  );

  const createProjectFn = useCallback(
    async (name: string) => {
      const p = await api.createProject(name);
      setOpenError(null);
      setProject(p);
      navigate(appPath(p.id, p.views[0]?.id));
    },
    [navigate],
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
      navigate(appPath(p.id, view.id));
      return view;
    },
    [requireProject, navigate],
  );

  const deleteViewFn = useCallback(
    async (vid: string) => {
      const p = requireProject();
      await api.deleteView(p.id, vid);
      // If it was the open view, the URL is sent to another one (see below).
      setProject((prev) =>
        prev ? { ...prev, views: prev.views.filter((v) => v.id !== vid) } : prev,
      );
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
      const { inventory } = await api.generateInventory(p.id, vid, i18n.language);
      setProject((prev) =>
        prev ? replaceView(prev, vid, (v) => ({ ...v, inventory })) : prev,
      );
      return inventory;
    },
    [requireProject, i18n.language],
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
      const renders = await api.renderView(p.id, vid, req, announcingJob(opts));
      setProject((prev) =>
        prev ? replaceView(prev, vid, (v) => withRenders(v, renders)) : prev,
      );
      return renders;
    },
    [requireProject],
  );

  const editRenderFn = useCallback(
    async (
      vid: string,
      rid: string,
      regions: EditRegionRequest[],
      opts?: { onProgress?: (job: RenderJob) => void; signal?: AbortSignal },
    ) => {
      const p = requireProject();
      const renders = await api.editRender(p.id, vid, rid, regions, announcingJob(opts));
      setProject((prev) =>
        prev ? replaceView(prev, vid, (v) => withRenders(v, renders)) : prev,
      );
      return renders;
    },
    [requireProject],
  );

  const upscaleRenderFn = useCallback(
    async (
      vid: string,
      rid: string,
      opts?: { onProgress?: (job: RenderJob) => void; signal?: AbortSignal },
    ) => {
      const p = requireProject();
      const renders = await api.upscaleRender(p.id, vid, rid, announcingJob(opts));
      setProject((prev) =>
        prev ? replaceView(prev, vid, (v) => withRenders(v, renders)) : prev,
      );
      return renders;
    },
    [requireProject],
  );

  // The view the URL names, or the first one when it names none (or one that
  // is gone): the URL is corrected to match below.
  const selectedView = useMemo(
    () => project?.views.find((v) => v.id === route.vid) ?? project?.views[0] ?? null,
    [project, route.vid],
  );
  const selectedViewId = selectedView?.id ?? null;

  // The render the URL names. While one the project does not have yet is being
  // looked for on the server, none is shown rather than the wrong one.
  const ridKey = project && route.rid ? `${project.id}/${route.rid}` : null;
  const ridFound = !!route.rid && !!selectedView?.renders.some((r) => r.id === route.rid);
  const selectedRenderId = ridFound
    ? route.rid
    : ridKey && settledRid !== ridKey
      ? null
      : (selectedView?.renders.at(-1)?.id ?? null);

  // Keep the URL naming what is shown: a project's first view and a view's
  // latest render when it names none, and a render that does not exist (after
  // looking for it on the server, where a job that ran while this screen was
  // closed left it) falls back the same way.
  useEffect(() => {
    if (!project) return;
    const view = project.views.find((v) => v.id === route.vid);
    if (!view) {
      const first = project.views[0];
      if (first) navigate(appPath(project.id, first.id), { replace: true });
      return;
    }
    if (route.rid && view.renders.some((r) => r.id === route.rid)) return;
    if (route.rid && ridKey && settledRid !== ridKey) {
      if (fetchingRid.current !== ridKey) {
        fetchingRid.current = ridKey;
        void refreshProjectFn().finally(() => setSettledRid(ridKey));
      }
      return;
    }
    const latest = view.renders.at(-1);
    if (latest) navigate(appPath(project.id, view.id, latest.id), { replace: true });
  }, [project, route.vid, route.rid, ridKey, settledRid, navigate, refreshProjectFn]);

  // The latest route and selection, for callbacks that outlive a render (a
  // render finishing minutes after it started).
  const shown = useRef({ pid: route.pid, vid: selectedViewId });
  useEffect(() => {
    shown.current = { pid: route.pid, vid: selectedViewId };
  }, [route.pid, selectedViewId]);

  const selectViewFn = useCallback(
    (vid: string) => {
      if (route.pid) navigate(appPath(route.pid, vid));
    },
    [route.pid, navigate],
  );

  const selectRenderFn = useCallback(
    (vid: string, rid: string) => {
      const { pid, vid: current } = shown.current;
      if (pid && current === vid) navigate(appPath(pid, vid, rid));
    },
    [navigate],
  );

  const value: ProjectContextValue = {
    project,
    openingProjectId: route.pid && !project ? route.pid : null,
    openError,
    refreshProject: refreshProjectFn,
    me,
    meLoading,
    refreshMe: refreshMeFn,
    adjustDisplayedCredits: adjustDisplayedCreditsFn,
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
    selectView: selectViewFn,
    selectedRenderId,
    selectRender: selectRenderFn,
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
    editRender: editRenderFn,
    upscaleRender: upscaleRenderFn,
  };

  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

export function useProject(): ProjectContextValue {
  const ctx = useContext(ProjectContext);
  if (!ctx) throw new Error("useProject must be used within ProjectProvider");
  return ctx;
}
