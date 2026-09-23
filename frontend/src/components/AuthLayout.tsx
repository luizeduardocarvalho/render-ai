import type { ReactNode } from "react";

/**
 * Centers a Clerk drop-in component (SignIn/SignUp) on the app background,
 * matching the spacing/branding of the create-project screen so the auth
 * flow feels like part of render-ai rather than a bolted-on Clerk page.
 */
export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="auth-screen">
      <div className="auth-screen-brand">render-ai</div>
      {children}
    </div>
  );
}
