import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { ClerkProvider } from "@clerk/clerk-react";
import "./index.css";
import { ClerkMissingNotice } from "./components/ClerkMissingNotice.tsx";
import { GatedApp } from "./components/GatedApp.tsx";
import { SignInPage } from "./components/SignInPage.tsx";
import { SignUpPage } from "./components/SignUpPage.tsx";
import { clerkAppearance } from "./lib/clerkAppearance.ts";

const CLERK_PUBLISHABLE_KEY = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {CLERK_PUBLISHABLE_KEY ? (
      <ClerkProvider publishableKey={CLERK_PUBLISHABLE_KEY} appearance={clerkAppearance}>
        <BrowserRouter>
          <Routes>
            <Route path="/sign-in/*" element={<SignInPage />} />
            <Route path="/sign-up/*" element={<SignUpPage />} />
            <Route path="/*" element={<GatedApp />} />
          </Routes>
        </BrowserRouter>
      </ClerkProvider>
    ) : (
      <ClerkMissingNotice />
    )}
  </StrictMode>,
);
