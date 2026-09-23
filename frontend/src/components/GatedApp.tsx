import { RedirectToSignIn, SignedIn, SignedOut, useUser } from "@clerk/clerk-react";
import App from "../App";
import { NotAuthorized } from "./NotAuthorized";
import { ProjectProvider } from "../state/ProjectContext";

/**
 * Renders the real app for signed-in admins, a "not approved" screen for
 * signed-in non-admins, and bounces everyone else to /sign-in.
 *
 * This is a UX-only gate - the backend independently enforces admin access
 * (403s non-admin requests) - but a non-admin who signs in should see a
 * clean message here, never the app shell itself.
 */
export function GatedApp() {
  return (
    <>
      <SignedIn>
        <SignedInGate />
      </SignedIn>
      <SignedOut>
        <RedirectToSignIn />
      </SignedOut>
    </>
  );
}

/** Split out so useUser() (a hook) is only called while actually signed in. */
function SignedInGate() {
  const { user } = useUser();
  const isAdmin = user?.publicMetadata?.role === "admin";

  if (!isAdmin) {
    return <NotAuthorized />;
  }

  return (
    <ProjectProvider>
      <App />
    </ProjectProvider>
  );
}
