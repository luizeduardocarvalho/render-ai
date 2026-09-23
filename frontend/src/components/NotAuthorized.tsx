import { UserButton, useUser } from "@clerk/clerk-react";
import { AuthLayout } from "./AuthLayout";

/**
 * Shown to a signed-in Clerk user whose publicMetadata.role is not "admin".
 * The app is gated to admins only - everyone else lands here instead of the
 * real app, with a way to see which account they're signed in as and sign out.
 */
export function NotAuthorized() {
  const { user } = useUser();
  const email = user?.primaryEmailAddress?.emailAddress;

  return (
    <AuthLayout>
      <div className="panel not-authorized-card">
        <h1 className="not-authorized-heading">Access not approved yet</h1>
        <p className="field-hint">
          This account isn't authorized for render-ai yet. Ask the team to grant you access,
          then sign in again.
        </p>
        {email && (
          <p className="not-authorized-email">
            Signed in as <strong>{email}</strong>
          </p>
        )}
        <div className="not-authorized-actions">
          <UserButton afterSignOutUrl="/sign-in" />
        </div>
      </div>
    </AuthLayout>
  );
}
