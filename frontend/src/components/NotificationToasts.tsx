import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useNotifications } from "../state/notificationsContext";
import type { JobNotification } from "../types";
import { AlertCircleIcon, CheckCircleIcon, CloseIcon } from "./NotificationIcons";
import "./Notifications.css";

const TOAST_MS = 8_000;

function Toast({ n }: { n: JobNotification }) {
  const { t } = useTranslation();
  const { open, dismissToast } = useNotifications();

  useEffect(() => {
    const timer = window.setTimeout(() => dismissToast(n.jobId), TOAST_MS);
    return () => window.clearTimeout(timer);
  }, [n.jobId, dismissToast]);

  const failed = n.status === "failed";
  const kind = t(`notifications.kind.${n.kind}`);
  return (
    <div className={`notif-toast ${failed ? "notif-toast-failed" : "notif-toast-done"}`}>
      <button type="button" className="notif-toast-main" onClick={() => open(n)}>
        <span className="notif-row-icon">{failed ? <AlertCircleIcon /> : <CheckCircleIcon />}</span>
        <span className="notif-row-text">
          <span className="notif-row-title">
            {t(failed ? "notifications.alert.failed" : "notifications.alert.done", { kind })}
          </span>
          <span className="notif-row-meta">
            {n.projectName} / {n.viewName}
          </span>
        </span>
      </button>
      <button
        type="button"
        className="notif-toast-close"
        aria-label={t("notifications.dismiss")}
        onClick={() => dismissToast(n.jobId)}
      >
        <CloseIcon />
      </button>
    </div>
  );
}

/** Finished jobs announced while the user is somewhere else in the app. */
export function NotificationToasts() {
  const { toasts } = useNotifications();
  return (
    <div className="notif-toasts" role="status" aria-live="polite">
      {toasts.map((n) => (
        <Toast key={n.jobId} n={n} />
      ))}
    </div>
  );
}
