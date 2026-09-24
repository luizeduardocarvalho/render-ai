import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import "./i18n";
import { AppRoot } from "./AppRoot.tsx";
import { ClerkMissingNotice } from "./components/ClerkMissingNotice.tsx";

const CLERK_PUBLISHABLE_KEY = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {CLERK_PUBLISHABLE_KEY ? (
      <AppRoot clerkPublishableKey={CLERK_PUBLISHABLE_KEY} />
    ) : (
      <ClerkMissingNotice />
    )}
  </StrictMode>,
);
