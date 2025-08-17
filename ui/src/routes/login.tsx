import React, { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "react-hot-toast";
import AuthLayout from "../components/AuthLayout";
import { Button } from "../components/Button";
import { InputField } from "../components/InputField";
import { useAuthStore } from "../hooks/useAuthStore";

export default function LoginRoute() {
  const [sq] = useSearchParams();
  const navigate = useNavigate();
  const { user, isLoading, login } = useAuthStore();
  
  const [formData, setFormData] = useState({
    username: "",
    password: "",
  });
  const [errors, setErrors] = useState<{ username?: string; password?: string }>({});

  // If already logged in, redirect to dashboard or return URL
  if (user?.isAuthenticated) {
    const returnTo = sq.get("returnTo") || "/";
    return <Navigate to={returnTo} replace />;
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Basic validation
    const newErrors: { username?: string; password?: string } = {};
    if (!formData.username.trim()) {
      newErrors.username = "Username is required";
    }
    if (!formData.password.trim()) {
      newErrors.password = "Password is required"; // eslint-disable-line sonarjs/no-hardcoded-passwords
    }
    
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    
    setErrors({});
    
    try {
      const success = await login(formData.username, formData.password);
      
      if (success) {
        toast.success("Login successful!");
        const returnTo = sq.get("returnTo") || "/";
        navigate(returnTo, { replace: true });
      } else {
        toast.error("Invalid username or password");
      }
    } catch (error) {
      console.error("Login error:", error);
      toast.error("Login failed. Please try again.");
    }
  };

  const handleInputChange = (field: keyof typeof formData) => (
    e: React.ChangeEvent<HTMLInputElement>
  ) => {
    setFormData(prev => ({ ...prev, [field]: e.target.value }));
    // Clear error when user starts typing
    if (errors[field]) {
      setErrors(prev => ({ ...prev, [field]: undefined }));
    }
  };

  return (
    <AuthLayout
      title="Log in to VideoNode"
      description="Access your video streaming and device management platform"
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        <InputField
          label="Username"
          type="text"
          value={formData.username}
          onChange={handleInputChange("username")}
          {...(errors.username && { error: errors.username })}
          placeholder="Enter your username"
          fullWidth
          autoComplete="username"
          disabled={isLoading}
        />
        
        <InputField
          label="Password"
          type="password"
          value={formData.password}
          onChange={handleInputChange("password")}
          {...(errors.password && { error: errors.password })}
          placeholder="Enter your password"
          fullWidth
          autoComplete="current-password"
          disabled={isLoading}
        />
        
        <Button
          type="submit"
          size="LG"
          theme="primary"
          fullWidth
          text="Log in"
          textAlign="center"
          loading={isLoading}
          disabled={isLoading}
        />
      </form>
    </AuthLayout>
  );
}