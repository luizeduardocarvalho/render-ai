// Minimal typing for the global `window.Clerk` object that Clerk's script
// installs once loaded. We only need the session token getter here - full
// typings live in @clerk/clerk-react but aren't attached to `window`.
declare global {
  interface Window {
    Clerk?: {
      session?: {
        getToken: () => Promise<string | null>;
      };
    };
  }
}

/**
 * Reads the current Clerk session token, if any. Returns null when Clerk
 * hasn't loaded yet, there is no active session, or the token fetch fails -
 * callers should treat a null token as "send the request unauthenticated".
 */
export async function getClerkToken(): Promise<string | null> {
  try {
    const token = await window.Clerk?.session?.getToken();
    return token ?? null;
  } catch {
    return null;
  }
}
