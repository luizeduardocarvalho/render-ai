import { useTranslation } from "react-i18next";

/** Formats an ISO timestamp as "just now", "5m ago", "3h ago" or a date. */
export function useRelativeTime() {
  const { t, i18n } = useTranslation();
  return function relativeTime(iso: string): string {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return "";
    const diff = Date.now() - then;
    const mins = Math.round(diff / 60000);
    if (mins < 1) return t("picker.time.justNow");
    if (mins < 60) return t("picker.time.minutesAgo", { count: mins });
    const hrs = Math.round(mins / 60);
    if (hrs < 24) return t("picker.time.hoursAgo", { count: hrs });
    const days = Math.round(hrs / 24);
    if (days < 30) return t("picker.time.daysAgo", { count: days });
    return new Date(iso).toLocaleDateString(i18n.language);
  };
}
