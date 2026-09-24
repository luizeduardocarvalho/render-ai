import { BrowserRouter, Route, Routes } from "react-router-dom";
import { ClerkProvider } from "@clerk/clerk-react";
import { ptBR, enUS } from "@clerk/localizations";
import { useTranslation } from "react-i18next";
import { AdminGate } from "./components/AdminGate";
import { GatedApp } from "./components/GatedApp";
import { SignInPage } from "./components/SignInPage";
import { SignUpPage } from "./components/SignUpPage";
import { clerkAppearance } from "./lib/clerkAppearance";

/** Picks Clerk's own drop-in UI localization to match the app's active language. */
export function AppRoot({ clerkPublishableKey }: { clerkPublishableKey: string }) {
  const { i18n } = useTranslation();
  const clerkLocalization = i18n.language === "en" ? enUS : ptBR;

  return (
    <ClerkProvider
      publishableKey={clerkPublishableKey}
      appearance={clerkAppearance}
      localization={clerkLocalization}
    >
      <BrowserRouter>
        <Routes>
          <Route path="/sign-in/*" element={<SignInPage />} />
          <Route path="/sign-up/*" element={<SignUpPage />} />
          <Route path="/admin" element={<AdminGate />} />
          <Route path="/*" element={<GatedApp />} />
        </Routes>
      </BrowserRouter>
    </ClerkProvider>
  );
}
