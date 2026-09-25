import type { JobNotification } from "../types";
import { appPath } from "./routes";

/** Still queued or running: no outcome yet. */
export function isActive(n: JobNotification): boolean {
  return n.status === "queued" || n.status === "running";
}

/** Finished (either way) and not yet opened by the user. */
export function isUnread(n: JobNotification): boolean {
  return !isActive(n) && !n.seen;
}

/**
 * Where opening a notification goes: the render it made once it is done, else
 * the view it is for (still running, or failed, so there is no render).
 */
export function notificationPath(n: JobNotification): string {
  const rid = n.status === "done" ? n.renderId : undefined;
  return appPath(n.projectId, n.viewId, rid);
}
