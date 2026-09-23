/**
 * Maps the app's CSS custom properties (see src/index.css) onto Clerk's
 * appearance API so the drop-in <SignIn>/<SignUp>/<UserButton> components
 * match the rest of render-ai instead of Clerk's default look. Using CSS
 * vars (rather than resolved colors) means this adapts to light/dark
 * automatically via the existing `prefers-color-scheme` rules.
 */
export const clerkAppearance = {
  variables: {
    colorPrimary: "var(--accent)",
    colorBackground: "var(--surface)",
    colorText: "var(--text)",
    colorTextSecondary: "var(--text-muted)",
    colorInputBackground: "var(--surface)",
    colorInputText: "var(--text)",
    colorDanger: "var(--danger)",
    colorSuccess: "var(--success)",
    colorWarning: "var(--warning)",
    colorNeutral: "var(--text-muted)",
    borderRadius: "var(--radius-md)",
    fontFamily: "var(--sans)",
    fontSize: "14px",
  },
  elements: {
    card: {
      boxShadow: "var(--shadow-lg)",
      border: "1px solid var(--border)",
      borderRadius: "var(--radius-lg)",
    },
    headerTitle: {
      color: "var(--text)",
    },
    headerSubtitle: {
      color: "var(--text-muted)",
    },
    socialButtonsBlockButton: {
      borderColor: "var(--border-strong)",
      borderRadius: "var(--radius-sm)",
    },
    formButtonPrimary: {
      backgroundColor: "var(--accent)",
      borderRadius: "var(--radius-sm)",
      fontSize: "13px",
      "&:hover": {
        backgroundColor: "var(--accent-hover)",
      },
    },
    formFieldInput: {
      borderColor: "var(--border-strong)",
      borderRadius: "var(--radius-sm)",
      backgroundColor: "var(--surface)",
      color: "var(--text)",
    },
    footerActionLink: {
      color: "var(--accent)",
    },
    dividerLine: {
      backgroundColor: "var(--border)",
    },
    dividerText: {
      color: "var(--text-faint)",
    },
  },
};
