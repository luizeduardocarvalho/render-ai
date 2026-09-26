import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

type Theme = "light" | "dark";

function systemTheme(): Theme {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/**
 * Round icon button that forces a light/dark theme via data-theme on <html>,
 * saved under the same localStorage key ("theme") the landing page uses.
 * index.html applies a saved choice before first paint to avoid a flash.
 *
 * Only an explicit click writes data-theme/localStorage: merely rendering
 * must not pin the system's current preference as a permanent override, or
 * "follow the system" would stop working after the first visit.
 */
export function ThemeToggle() {
  const { t } = useTranslation();
  const saved = document.documentElement.dataset.theme;
  const [effective, setEffective] = useState<Theme>(saved === "light" || saved === "dark" ? saved : systemTheme);

  // Follow the system while no theme has been chosen. Checked on every change,
  // so a choice made after mount (a click here, or in another toggle on the
  // page) stops the system from flipping the icon.
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      const chosen = document.documentElement.dataset.theme;
      if (chosen !== "light" && chosen !== "dark") setEffective(systemTheme());
    };
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, []);

  function choose(theme: Theme) {
    document.documentElement.dataset.theme = theme;
    try {
      window.localStorage.setItem("theme", theme);
    } catch {
      // Storage may be unavailable (private mode, quota); the toggle still
      // works for the current session.
    }
    setEffective(theme);
  }

  const target: Theme = effective === "dark" ? "light" : "dark";

  return (
    <button
      type="button"
      className="btn btn-icon theme-toggle"
      onClick={() => choose(target)}
      aria-label={target === "dark" ? t("theme.toDark") : t("theme.toLight")}
      title={target === "dark" ? t("theme.toDark") : t("theme.toLight")}
    >
      {effective === "dark" ? (
        <svg className="theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4" />
        </svg>
      ) : (
        <svg className="theme-icon" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z" />
        </svg>
      )}
    </button>
  );
}
