import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { resources } from "./resources";

export const SUPPORTED_LANGUAGES = ["pt-BR", "en"] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];

const STORAGE_KEY = "render-ai:language";
const DEFAULT_LANGUAGE: SupportedLanguage = "pt-BR";

function readStoredLanguage(): SupportedLanguage | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored === "pt-BR" || stored === "en" ? stored : null;
  } catch {
    // Private browsing / blocked storage - fall through to detection.
    return null;
  }
}

// Most users are in Brazil, so the browser-language step only ever *confirms*
// Portuguese - it never auto-selects English. English is only used once the
// user explicitly picks it (persisted to localStorage); everyone else,
// regardless of OS/browser locale, starts in pt-BR.
function detectFromBrowser(): SupportedLanguage {
  const languages = navigator.languages && navigator.languages.length > 0 ? navigator.languages : [navigator.language];
  const hasPortuguese = languages.some((lang) => lang.toLowerCase().startsWith("pt"));
  return hasPortuguese ? "pt-BR" : DEFAULT_LANGUAGE;
}

export function resolveInitialLanguage(): SupportedLanguage {
  return readStoredLanguage() ?? detectFromBrowser();
}

export function persistLanguage(lang: SupportedLanguage): void {
  try {
    localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    // ignore storage failures (private mode, quota) - selection still works
    // for the rest of this session.
  }
}

export function applyHtmlLang(lang: SupportedLanguage): void {
  document.documentElement.lang = lang;
}

const initialLanguage = resolveInitialLanguage();
applyHtmlLang(initialLanguage);

void i18n
  .use(initReactI18next)
  .init({
    resources,
    lng: initialLanguage,
    fallbackLng: "pt-BR",
    supportedLngs: SUPPORTED_LANGUAGES,
    interpolation: {
      escapeValue: false, // React already escapes output.
    },
    returnNull: false,
  });

i18n.on("languageChanged", (lang) => {
  if (lang === "pt-BR" || lang === "en") {
    applyHtmlLang(lang);
  }
});

export default i18n;
