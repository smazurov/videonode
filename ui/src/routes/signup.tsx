import AuthLayout from "../components/AuthLayout";
import { LinkButton } from "../components/Button";

export default function SignupRoute() {
  return (
    <AuthLayout
      title="Sign up for VideoNode"
      description="Create an account to access the video streaming platform"
      showNavbar={false}
    >
      <div className="text-center space-y-4">
        <p className="text-gray-600 dark:text-gray-300">
          Account creation is currently managed by your system administrator.
        </p>
        <LinkButton
          to="/login"
          text="Back to Login"
          theme="primary"
          size="LG"
          fullWidth
          textAlign="center"
        />
      </div>
    </AuthLayout>
  );
}