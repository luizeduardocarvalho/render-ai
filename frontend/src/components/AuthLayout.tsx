import type { ReactNode } from "react";
import { BrandMark } from "./BrandMark";
import { ThemeToggle } from "./ThemeToggle";

/**
 * Centers a Clerk drop-in component (SignIn/SignUp) on the app background,
 * matching the spacing/branding of the create-project screen so the auth
 * flow feels like part of StudioIA rather than a bolted-on Clerk page. The
 * texture band is the one place the brand's full-color texture is welcome
 * inside the app (see brand/studio3d/README.md).
 */
export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="auth-screen">
      <div className="auth-screen-texture" aria-hidden="true" />
      <div className="auth-screen-toggle">
        <ThemeToggle />
      </div>
      <BrandMark className="auth-screen-brand" />
      {children}
    </div>
  );
}
