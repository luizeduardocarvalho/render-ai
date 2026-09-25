import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { useRelativeTime } from "../hooks/useRelativeTime";
import { isActive, isUnread, notificationPath } from "../lib/notifications";
import { useNotifications } from "../state/notificationsContext";
import type { JobNotification } from "../types";
import { AlertCircleIcon, BellIcon, CheckCircleIcon } from "./NotificationIcons";
import "./Notifications.css";

function StatusIcon({ n }: { n: JobNotification }) {
  if (isActive(n)) return <span className="spinner notif-row-icon" aria-hidden="true" />;
  if (n.status === "failed") {
    return (
      <span className="notif-row-icon notif-row-icon-failed">
        <AlertCircleIcon />
      </span>
    );
  }
  return (
    <span className="notif-row-icon notif-row-icon-done">
      <CheckCircleIcon />
    </span>
  );
}

/**
 * The bell in the app header: the user's renders, edits and upscales that are
 * running or finished in the last day. Each entry opens the project, view and
 * render it is about.
 */
export function NotificationsBell() {
  const { t, i18n } = useTranslation();
  const relativeTime = useRelativeTime();
  const {
    items,
    loaded,
    activeCount,
    unreadCount,
    markSeen,
    markAllSeen,
    desktopPermission,
    enableDesktopAlerts,
  } = useNotifications();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelId = useId();
  // Re-render while the panel is open so "just now" and "5m ago" keep up.
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!open) return;
    const timer = window.setInterval(() => setTick((n) => n + 1), 30_000);
    return () => window.clearInterval(timer);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpen(false);
      buttonRef.current?.focus();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const running = items.filter(isActive);
  const recent = items.filter((n) => !isActive(n));

  const label = [
    t("notifications.title"),
    unreadCount > 0 ? t("notifications.bell.unread", { count: unreadCount }) : null,
    activeCount > 0 ? t("notifications.bell.running", { count: activeCount }) : null,
  ]
    .filter(Boolean)
    .join(", ");

  function statusText(n: JobNotification): string {
    if (n.status === "queued") return t("notifications.status.queued");
    if (n.status === "running") {
      const { done, failed, total } = n.variations;
      return total > 1
        ? t("notifications.status.runningProgress", { done: done + failed, count: total })
        : t("notifications.status.running");
    }
    return n.status === "failed" ? t("notifications.status.failed") : t("notifications.status.done");
  }

  function renderRow(n: JobNotification) {
    const unread = isUnread(n);
    return (
      <li key={n.jobId}>
        <Link
          to={notificationPath(n)}
          className={`notif-row ${unread ? "notif-row-unread" : ""}`}
          onClick={() => {
            markSeen(n);
            setOpen(false);
          }}
        >
          <StatusIcon n={n} />
          <span className="notif-row-text">
            <span className="notif-row-title">
              {t(`notifications.kind.${n.kind}`)} · {n.viewName}
            </span>
            <span className="notif-row-meta">
              {n.projectName} · {statusText(n)}
            </span>
          </span>
          <span className="notif-row-side">
            <time
              className="notif-row-time"
              dateTime={n.createdAt}
              title={new Date(n.createdAt).toLocaleString(i18n.language)}
            >
              {relativeTime(n.createdAt)}
            </time>
            {unread && (
              <>
                <span className="notif-dot" aria-hidden="true" />
                <span className="visually-hidden">{t("notifications.unreadMark")}</span>
              </>
            )}
          </span>
        </Link>
      </li>
    );
  }

  return (
    <div className="notif-root" ref={rootRef}>
      <button
        ref={buttonRef}
        type="button"
        className={`notif-bell ${activeCount > 0 ? "notif-bell-active" : ""}`}
        aria-label={label}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => setOpen((v) => !v)}
      >
        <BellIcon />
        {unreadCount > 0 && (
          <span className="notif-badge" aria-hidden="true">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="notif-panel" id={panelId} role="region" aria-label={t("notifications.title")}>
          <div className="notif-panel-head">
            <strong>{t("notifications.title")}</strong>
            {unreadCount > 0 && (
              <button type="button" className="btn btn-ghost btn-sm" onClick={markAllSeen}>
                {t("notifications.markAllRead")}
              </button>
            )}
          </div>

          <div className="notif-panel-body">
            {!loaded && (
              <div className="notif-empty">
                <span className="spinner" /> {t("notifications.loading")}
              </div>
            )}

            {loaded && items.length === 0 && (
              <div className="notif-empty notif-empty-stack">
                <strong>{t("notifications.empty.title")}</strong>
                <span>{t("notifications.empty.body")}</span>
              </div>
            )}

            {running.length > 0 && (
              <section aria-label={t("notifications.sections.running")}>
                <h3 className="notif-section">{t("notifications.sections.running")}</h3>
                <ul className="notif-list">{running.map(renderRow)}</ul>
              </section>
            )}

            {recent.length > 0 && (
              <section aria-label={t("notifications.sections.recent")}>
                <h3 className="notif-section">{t("notifications.sections.recent")}</h3>
                <ul className="notif-list">{recent.map(renderRow)}</ul>
              </section>
            )}
          </div>

          {desktopPermission === "default" && (
            <div className="notif-desktop">
              <span>{t("notifications.desktop.prompt")}</span>
              <button type="button" className="btn btn-sm" onClick={() => void enableDesktopAlerts()}>
                {t("notifications.desktop.enable")}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
