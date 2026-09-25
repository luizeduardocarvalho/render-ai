import type { TFunction } from "i18next";
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

/** The headline of the alert for a finished job: "Edit finished", "Render failed". */
export function alertTitle(t: TFunction, n: JobNotification): string {
  return n.status === "failed"
    ? t("notifications.alert.failed", { kind: t(`notifications.kind.${n.kind}`) })
    : t(`notifications.alert.done.${n.kind}`);
}

/**
 * The jobs in `next` that were running (or queued) in `known` and are finished
 * now: the ones to alert about. A job first seen already finished (the app just
 * opened) is not one of them - it is only an unread entry.
 */
export function justFinished(
  known: ReadonlyMap<string, JobNotification>,
  next: readonly JobNotification[],
): JobNotification[] {
  return next.filter((n) => {
    const before = known.get(n.jobId);
    return before !== undefined && isActive(before) && !isActive(n);
  });
}
