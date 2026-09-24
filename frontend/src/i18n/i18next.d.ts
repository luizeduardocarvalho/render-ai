import "i18next";
import en from "./locales/en.json";

// Makes t("some.key") a type error at compile time when the key doesn't
// exist in en.json (the canonical key set - pt-BR.json is required to carry
// the exact same keys, enforced by scripts/check-i18n.mjs).
declare module "i18next" {
  interface CustomTypeOptions {
    defaultNS: "translation";
    resources: {
      translation: typeof en;
    };
  }
}
