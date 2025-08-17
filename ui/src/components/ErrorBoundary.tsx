import React from "react";
import { useRouteError, isRouteErrorResponse } from "react-router-dom";
import { ExclamationTriangleIcon } from "@heroicons/react/16/solid";

// TypeScript interfaces for proper error typing
interface ErrorWithMessage {
  message: string;
}

interface ErrorWithData {
  data: {
    error?: {
      message?: string;
    };
  };
}

// Type guards for error narrowing
function isErrorWithMessage(error: unknown): error is ErrorWithMessage {
  return (
    typeof error === "object" &&
    error !== null &&
    "message" in error &&
    typeof (error as Record<string, unknown>).message === "string"
  );
}

function isErrorWithData(error: unknown): error is ErrorWithData {
  return (
    typeof error === "object" &&
    error !== null &&
    "data" in error &&
    typeof (error as Record<string, unknown>).data === "object" &&
    (error as Record<string, unknown>).data !== null
  );
}

function getErrorMessage(error: unknown): string {
  if (isRouteErrorResponse(error)) {
    if (error.data && typeof error.data === 'object' && 'message' in error.data && typeof (error.data as Record<string, unknown>).message === 'string') {
      return (error.data as { message: string }).message;
    }
    return error.statusText;
  }

  if (isErrorWithData(error) && error.data.error?.message) {
    return error.data.error.message;
  }

  if (isErrorWithMessage(error)) {
    return error.message;
  }

  if (error instanceof Error) {
    return error.message;
  }

  return 'An unknown error occurred';
}

// Error boundary component with proper TypeScript typing
export default function ErrorBoundary(): React.JSX.Element {
  const error = useRouteError();
  const errorMessage = getErrorMessage(error);

  // Handle 404 errors specifically
  if (isRouteErrorResponse(error) && error.status === 404) {
    return (
      <div className="h-full w-full flex items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-4">Page Not Found</h1>
          <p className="text-gray-600 dark:text-gray-300">
            The page you're looking for doesn't exist.
          </p>
        </div>
      </div>
    );
  }

  // Handle other route errors with status codes
  if (isRouteErrorResponse(error)) {
    return (
      <div className="h-full w-full">
        <div className="flex h-full items-center justify-center">
          <div className="w-full max-w-2xl text-center">
            <ExclamationTriangleIcon className="h-12 w-12 mx-auto mb-4 text-red-500" />
            <h1 className="text-2xl font-bold mb-4">
              {error.status} {error.statusText}
            </h1>
            <p className="text-gray-600 dark:text-gray-300 mb-4">
              {errorMessage}
            </p>
          </div>
        </div>
      </div>
    );
  }

  // Handle generic errors
  return (
    <div className="h-full w-full">
      <div className="flex h-full items-center justify-center">
        <div className="w-full max-w-2xl text-center">
          <ExclamationTriangleIcon className="h-12 w-12 mx-auto mb-4 text-red-500" />
          <h1 className="text-2xl font-bold mb-4">Oh no!</h1>
          <p className="text-gray-600 dark:text-gray-300 mb-4">
            Something went wrong. Please try again later.
          </p>
          <div className="bg-gray-100 dark:bg-gray-800 p-4 rounded-md">
            <code className="text-sm text-red-600 dark:text-red-400">
              {errorMessage}
            </code>
          </div>
        </div>
      </div>
    </div>
  );
}