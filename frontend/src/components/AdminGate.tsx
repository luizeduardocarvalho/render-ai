import { RedirectToSignIn, SignedIn, SignedOut } from "@clerk/clerk-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, getMe } from "../api";
import type { Me } from "../types";
import { AdminPage } from "./AdminPage";
import { AuthLayout } from "./AuthLayout";
import { NotAuthorized } from "./NotAuthorized";

/**
 * Gate for the /admin route: signed-out users are bounced to /sign-in like
 * everywhere else; signed-in users get the admin page only if GET /api/me
 * reports isAdmin. This is a UX-only gate - the backend independently
 * enforces requireAdmin on every /api/admin/* route.
 */
export function AdminGate() {
  return (
    <>
      <SignedIn>
        <AdminSignedInGate />
      </SignedIn>
      <SignedOut>
        <RedirectToSignIn />
      </SignedOut>
    </>
  );
}

function AdminSignedInGate() {
  const { t } = useTranslation();
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    getMe()
      .then((m) => {
        if (active) setMe(m);
      })
      .catch((err) => {
        if (active) setError(err instanceof ApiError ? err.message : t("admin.errors.loadMe"));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [t]);

  if (loading) {
    return (
      <AuthLayout>
        <div className="picker-status">
          <span className="spinner" /> {t("admin.loading")}
        </div>
      </AuthLayout>
    );
  }

  if (error || !me?.isAdmin) {
    return <NotAuthorized />;
  }

  return <AdminPage />;
}
