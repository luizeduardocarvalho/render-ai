import en from "./locales/en.json";
import ptBR from "./locales/pt-BR.json";

// English is the canonical key set used for TypeScript's `t()` checking (see
// i18next.d.ts). scripts/check-i18n.mjs enforces at build/CI time that
// pt-BR.json carries exactly the same keys, so the two never drift apart.
export const resources = {
  en: { translation: en },
  "pt-BR": { translation: ptBR },
} as const;

export default resources;
