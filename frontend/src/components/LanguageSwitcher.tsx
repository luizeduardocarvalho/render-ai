import { useTranslation } from "react-i18next";
import { persistLanguage, type SupportedLanguage } from "../i18n";

/**
 * Small PT / EN toggle shown in the app header, next to the other header
 * controls. Reuses the app's existing `.segmented` control styling.
 */
export function LanguageSwitcher() {
  const { t, i18n } = useTranslation();
  const current = i18n.language === "en" ? "en" : "pt-BR";

  function select(lang: SupportedLanguage) {
    if (lang === current) return;
    void i18n.changeLanguage(lang);
    persistLanguage(lang);
  }

  return (
    <div className="segmented lang-switcher" role="group" aria-label={t("languageSwitcher.label")}>
      <button
        type="button"
        className={`segmented-btn ${current === "pt-BR" ? "segmented-btn-active" : ""}`}
        onClick={() => select("pt-BR")}
        aria-pressed={current === "pt-BR"}
      >
        {t("languageSwitcher.pt")}
      </button>
      <button
        type="button"
        className={`segmented-btn ${current === "en" ? "segmented-btn-active" : ""}`}
        onClick={() => select("en")}
        aria-pressed={current === "en"}
      >
        {t("languageSwitcher.en")}
      </button>
    </div>
  );
}
