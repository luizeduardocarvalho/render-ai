/**
 * Maps the app's CSS custom properties (see src/index.css) onto Clerk's
 * appearance API so the drop-in <SignIn>/<SignUp>/<UserButton> components
 * match the rest of StudioIA - D-DIN, brand-black primary button, teal
 * focus/links, warm neutrals - instead of Clerk's default look. Using CSS
 * vars (rather than resolved colors) means this adapts to light/dark
 * automatically via the app's light-dark() tokens.
 */
export const clerkAppearance = {
  variables: {
    colorPrimary: "var(--btn-primary-bg)",
    colorBackground: "var(--surface)",
    colorText: "var(--text)",
    colorTextSecondary: "var(--text-muted)",
    colorInputBackground: "var(--surface)",
    colorInputText: "var(--text)",
    colorDanger: "var(--danger)",
    colorSuccess: "var(--success)",
    colorWarning: "var(--warning)",
    colorNeutral: "var(--text-muted)",
    borderRadius: "var(--radius-input)",
    fontFamily: "var(--sans)",
    fontSize: "15px",
  },
  elements: {
    card: {
      boxShadow: "var(--shadow-lg)",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius-lg)",
      background: "var(--surface-raised)",
    },
    headerTitle: {
      color: "var(--text)",
      fontWeight: 700,
    },
    headerSubtitle: {
      color: "var(--text-muted)",
    },
    socialButtonsBlockButton: {
      borderColor: "var(--border-strong)",
      borderRadius: "var(--radius-full)",
    },
    formButtonPrimary: {
      backgroundColor: "var(--btn-primary-bg)",
      color: "var(--btn-primary-ink)",
      borderRadius: "var(--radius-full)",
      fontSize: "14px",
      fontWeight: 700,
      "&:hover": {
        backgroundColor: "var(--btn-primary-hover)",
      },
      "&:focus": {
        boxShadow: "none",
      },
    },
    formFieldInput: {
      borderColor: "var(--border-strong)",
      borderRadius: "var(--radius-input)",
      backgroundColor: "var(--surface)",
      color: "var(--text)",
      "&:focus": {
        borderColor: "var(--accent)",
        boxShadow: "0 0 0 2px var(--accent-soft)",
      },
    },
    formFieldLabel: {
      color: "var(--text-faint)",
      fontSize: "12.5px",
      fontWeight: 700,
      textTransform: "uppercase",
      letterSpacing: "0.08em",
    },
    footerActionLink: {
      color: "var(--accent)",
      "&:hover": {
        color: "var(--accent-hover)",
      },
    },
    dividerLine: {
      backgroundColor: "var(--border)",
    },
    dividerText: {
      color: "var(--text-faint)",
    },
    identityPreviewText: {
      color: "var(--text)",
    },
    identityPreviewEditButton: {
      color: "var(--accent)",
    },
  },
};
