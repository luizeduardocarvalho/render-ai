import { RedirectToSignIn, SignedIn, SignedOut } from "@clerk/clerk-react";
import App from "../App";
import { NotificationsProvider } from "../state/NotificationsProvider";
import { ProjectProvider } from "../state/ProjectContext";
import { NotificationToasts } from "./NotificationToasts";

/**
 * Renders the real app for every signed-in user, and bounces everyone else
 * to /sign-in. There is no more admin-only gate here - any verified Clerk
 * session gets the app (rendering is metered by credits instead; see
 * CreditsChip / RenderControls). Only the separate /admin route stays
 * restricted, gated on isAdmin from GET /api/me (see AdminGate).
 */
export function GatedApp() {
  return (
    <>
      <SignedIn>
        <ProjectProvider>
          <NotificationsProvider>
            <App />
            <NotificationToasts />
          </NotificationsProvider>
        </ProjectProvider>
      </SignedIn>
      <SignedOut>
        <RedirectToSignIn />
      </SignedOut>
    </>
  );
}
