import { UserButton, useUser } from "@clerk/clerk-react";
import { useTranslation } from "react-i18next";
import { AuthLayout } from "./AuthLayout";

/**
 * Shown to a signed-in Clerk user whose publicMetadata.role is not "admin".
 * The app is gated to admins only - everyone else lands here instead of the
 * real app, with a way to see which account they're signed in as and sign out.
 */
export function NotAuthorized() {
  const { t } = useTranslation();
  const { user } = useUser();
  const email = user?.primaryEmailAddress?.emailAddress;

  return (
    <AuthLayout>
      <div className="panel not-authorized-card">
        <h1 className="not-authorized-heading">{t("auth.notAuthorized.heading")}</h1>
        <p className="field-hint">{t("auth.notAuthorized.body")}</p>
        {email && (
          <p className="not-authorized-email">
            {t("auth.notAuthorized.signedInAs")} <strong>{email}</strong>
          </p>
        )}
        <div className="not-authorized-actions">
          <UserButton afterSignOutUrl="/sign-in" />
        </div>
      </div>
    </AuthLayout>
  );
}
