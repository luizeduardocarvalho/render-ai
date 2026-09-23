import { SignIn } from "@clerk/clerk-react";
import { AuthLayout } from "./AuthLayout";

export function SignInPage() {
  return (
    <AuthLayout>
      <SignIn routing="path" path="/sign-in" signUpUrl="/sign-up" />
    </AuthLayout>
  );
}
