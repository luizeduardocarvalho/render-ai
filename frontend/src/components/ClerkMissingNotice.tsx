import { useTranslation } from "react-i18next";

export function ClerkMissingNotice() {
  const { t } = useTranslation();
  return (
    <div className="clerk-missing-screen">
      <p>{t("auth.clerkMissing")}</p>
    </div>
  );
}
