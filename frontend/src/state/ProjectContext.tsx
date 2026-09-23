import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import * as api from "../api";
import type {
  Asset,
  Mask,
  Project,
  Render,
  RenderRequest,
  StyleSettings,
  View,
} from "../types";

interface ProjectContextValue {
  project: Project | null;
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
  uploadAssetReference: (aid: string, file: File) => Promise<Asset>;

  createView: (name: string, file: File) => Promise<View>;
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

  renderView: (vid: string, req: RenderRequest) => Promise<Render[]>;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

function replaceView(project: Project, vid: string, updater: (v: View) => View): Project {
  return {
    ...project,
    views: project.views.map((v) => (v.id === vid ? updater(v) : v)),
  };
}

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [project, setProject] = useState<Project | null>(null);
  const [selectedViewId, setSelectedViewId] = useState<string | null>(null);

  const requireProject = useCallback(() => {
    if (!project) throw new Error("No active project");
    return project;
  }, [project]);

  const createProjectFn = useCallback(async (name: string) => {
    const p = await api.createProject(name);
    setProject(p);
    setSelectedViewId(p.views[0]?.id ?? null);
  }, []);

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
    async (aid: string, file: File) => {
      const p = requireProject();
      const asset = await api.uploadAssetReference(p.id, aid, file);
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
    async (name: string, file: File) => {
      const p = requireProject();
      const view = await api.createView(p.id, name, file);
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
    async (vid: string, req: RenderRequest) => {
      const p = requireProject();
      const renders = await api.renderView(p.id, vid, req);
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
