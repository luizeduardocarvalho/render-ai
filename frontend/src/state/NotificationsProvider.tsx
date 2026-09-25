import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import * as api from "../api";
import { onJobsChanged } from "../lib/jobEvents";
import { alertTitle, isActive, isUnread, justFinished, notificationPath } from "../lib/notifications";
import type { JobNotification } from "../types";
import { useProject } from "./ProjectContext";
import { NotificationsContext, type DesktopPermission, type NotificationsContextValue } from "./notificationsContext";

// How often the list is fetched while something is still running. With
// nothing running it is fetched only when the app opens, a job starts or ends
// here, or the user comes back to the tab.
const POLL_INTERVAL_MS = 5_000;

// The user is looking at the app: its tab is showing and its window has focus
// (a tab can be visible on a screen the user has turned away from).
function isAttending(): boolean {
  return document.visibilityState === "visible" && document.hasFocus();
}

function currentPermission(): DesktopPermission {
  return typeof Notification === "undefined" ? "unsupported" : Notification.permission;
}

export function NotificationsProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { project, selectedViewId, selectedRenderId, refreshProject } = useProject();

  const [items, setItems] = useState<JobNotification[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [toasts, setToasts] = useState<JobNotification[]>([]);
  const [desktopPermission, setDesktopPermission] = useState<DesktopPermission>(currentPermission);
  const [attending, setAttending] = useState(isAttending);

  // What the last list held, to tell when a job goes from running to finished.
  const known = useRef(new Map<string, JobNotification>());
  // Jobs marked seen here, so a list fetched before the server recorded it
  // cannot bring the unread dot back.
  const seenHere = useRef(new Set<string>());
  const requested = useRef(0);
  const applied = useRef(0);

  const pid = project?.id ?? null;
  const isShowing = useCallback(
    (n: JobNotification) => {
      if (!attending || n.projectId !== pid || n.viewId !== selectedViewId) return false;
      // A finished job's render must be the one on screen; a failed one made none.
      return n.status === "done" && n.renderId ? n.renderId === selectedRenderId : true;
    },
    [attending, pid, selectedViewId, selectedRenderId],
  );

  const openNotification = useCallback(
    (n: JobNotification) => navigate(notificationPath(n)),
    [navigate],
  );

  const alertFinished = useCallback(
    (n: JobNotification) => {
      // The toast is always queued: it is what is waiting when the user comes
      // back to the tab (it only starts to count down once they are looking).
      setToasts((prev) => [n, ...prev.filter((x) => x.jobId !== n.jobId)].slice(0, 3));
      // Someone who is not looking at the app also gets a desktop notification,
      // if they have allowed those.
      if (isAttending() || currentPermission() !== "granted") return;
      try {
        const desktop = new Notification(alertTitle(t, n), {
          body: `${n.projectName} / ${n.viewName}`,
          tag: n.jobId,
        });
        desktop.onclick = () => {
          window.focus();
          openNotification(n);
          desktop.close();
        };
      } catch {
        // Some browsers (Chrome on Android) allow the permission but refuse
        // this constructor. The toast is there.
      }
    },
    [t, openNotification],
  );

  // The newest callbacks, for the list arriving asynchronously.
  const latest = useRef({ isShowing, alertFinished, refreshProject, pid });
  useEffect(() => {
    latest.current = { isShowing, alertFinished, refreshProject, pid };
  }, [isShowing, alertFinished, refreshProject, pid]);

  const apply = useCallback((list: JobNotification[]) => {
    const next = list.map((n) => (seenHere.current.has(n.jobId) ? { ...n, seen: true } : n));
    const finished = justFinished(known.current, next);
    known.current = new Map(next.map((n) => [n.jobId, n]));
    setItems(next);
    setLoaded(true);

    for (const n of finished) {
      // One job's alert failing must not cost the others theirs.
      try {
        const { isShowing, alertFinished, refreshProject, pid } = latest.current;
        // Its renders are in the open project's history now.
        if (n.projectId === pid) void refreshProject();
        if (!isShowing(n)) alertFinished(n);
      } catch {
        // Nothing to do: the entry is in the bell either way.
      }
    }
  }, []);

  const refresh = useCallback(async () => {
    const mine = ++requested.current;
    try {
      const list = await api.listMyRenderJobs();
      if (mine < applied.current) return; // a newer answer already arrived
      applied.current = mine;
      apply(list);
    } catch {
      // Keep the list already shown; the next poll tries again.
    }
  }, [apply]);

  useEffect(() => {
    void refresh();
    const stop = onJobsChanged(() => void refresh());
    // The list is fetched when the user comes back, once: coming back fires
    // both "focus" and "visibilitychange".
    let wasAttending = isAttending();
    const onAttention = () => {
      const now = isAttending();
      setAttending(now);
      if (now && !wasAttending) void refresh();
      wasAttending = now;
    };
    window.addEventListener("focus", onAttention);
    window.addEventListener("blur", onAttention);
    document.addEventListener("visibilitychange", onAttention);
    return () => {
      stop();
      window.removeEventListener("focus", onAttention);
      window.removeEventListener("blur", onAttention);
      document.removeEventListener("visibilitychange", onAttention);
    };
  }, [refresh]);

  const hasActive = items.some(isActive);
  useEffect(() => {
    if (!hasActive) return;
    const timer = window.setInterval(() => void refresh(), POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [hasActive, refresh]);

  const markSeen = useCallback(
    (n: JobNotification) => {
      // A job still running has no outcome to have seen (the server ignores it
      // too): remembering it as seen would hide its result when it arrives.
      if (isActive(n) || n.seen || seenHere.current.has(n.jobId)) return;
      seenHere.current.add(n.jobId);
      setItems((prev) => prev.map((x) => (x.jobId === n.jobId ? { ...x, seen: true } : x)));
      // Whatever it announced has been seen: take its toast down.
      setToasts((prev) => prev.filter((x) => x.jobId !== n.jobId));
      api.markRenderJobSeen(n.projectId, n.jobId).catch(() => {
        // Not recorded: let the server's answer decide again.
        seenHere.current.delete(n.jobId);
        void refresh();
      });
    },
    [refresh],
  );

  // Opening what a job made is seeing it.
  useEffect(() => {
    for (const n of items) {
      if (isUnread(n) && isShowing(n)) markSeen(n);
    }
  }, [items, isShowing, markSeen]);

  const markAllSeen = useCallback(() => {
    for (const n of items) if (isUnread(n)) markSeen(n);
  }, [items, markSeen]);

  const open = useCallback(
    (n: JobNotification) => {
      markSeen(n);
      openNotification(n);
    },
    [markSeen, openNotification],
  );

  const dismissToast = useCallback((jobId: string) => {
    setToasts((prev) => prev.filter((x) => x.jobId !== jobId));
  }, []);

  const enableDesktopAlerts = useCallback(async () => {
    if (typeof Notification === "undefined") return;
    setDesktopPermission(await Notification.requestPermission());
  }, []);

  const value = useMemo<NotificationsContextValue>(
    () => ({
      items,
      loaded,
      activeCount: items.filter(isActive).length,
      unreadCount: items.filter(isUnread).length,
      markSeen,
      markAllSeen,
      open,
      toasts,
      dismissToast,
      attending,
      desktopPermission,
      enableDesktopAlerts,
    }),
    [
      items,
      loaded,
      markSeen,
      markAllSeen,
      open,
      toasts,
      dismissToast,
      attending,
      desktopPermission,
      enableDesktopAlerts,
    ],
  );

  return (
    <NotificationsContext.Provider value={value}>
      {children}
    </NotificationsContext.Provider>
  );
}
