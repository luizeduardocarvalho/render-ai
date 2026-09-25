import { createContext, useContext } from "react";
import type { JobNotification } from "../types";

export type DesktopPermission = NotificationPermission | "unsupported";

export interface NotificationsContextValue {
  /** The user's render jobs from the last day, newest first. */
  items: JobNotification[];
  /** False until the first list has arrived. */
  loaded: boolean;
  activeCount: number;
  unreadCount: number;
  markSeen: (n: JobNotification) => void;
  markAllSeen: () => void;
  /** Where opening a notification goes. */
  open: (n: JobNotification) => void;
  /** Toasts for jobs that finished while the user was elsewhere in the app. */
  toasts: JobNotification[];
  dismissToast: (jobId: string) => void;
  /** The user is looking at the app: its tab is showing and its window focused. */
  attending: boolean;
  desktopPermission: DesktopPermission;
  enableDesktopAlerts: () => Promise<void>;
}

export const NotificationsContext = createContext<NotificationsContextValue | null>(null);

export function useNotifications(): NotificationsContextValue {
  const ctx = useContext(NotificationsContext);
  if (!ctx) throw new Error("useNotifications must be used within NotificationsProvider");
  return ctx;
}
