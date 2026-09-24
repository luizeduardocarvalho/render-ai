import { UserButton, useUser } from "@clerk/clerk-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { AuthLayout } from "./AuthLayout";

/**
 * Shown to a signed-in Clerk user who isn't an admin but landed on /admin.
 * Every signed-in user gets the main app now (see GatedApp) - only the admin
 * area stays restricted, so this is the sole remaining use of this screen.
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
          <Link to="/" className="btn btn-ghost btn-sm">
            {t("auth.notAuthorized.backToApp")}
          </Link>
          <UserButton afterSignOutUrl="/sign-in" />
        </div>
      </div>
    </AuthLayout>
  );
}
