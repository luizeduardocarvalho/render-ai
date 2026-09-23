import { SignUp } from "@clerk/clerk-react";
import { AuthLayout } from "./AuthLayout";

export function SignUpPage() {
  return (
    <AuthLayout>
      <SignUp routing="path" path="/sign-up" signInUrl="/sign-in" />
    </AuthLayout>
  );
}
