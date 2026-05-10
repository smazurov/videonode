import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { testAuth } from "../lib/api";
import { clearAuthCredentials, setAuthCredentials } from "../lib/auth";

export interface User {
  username: string;
  isAuthenticated: boolean;
}

interface AuthState {
  user: User | null;
  isLoading: boolean;
  setUser: (user: User | null) => void;
  setLoading: (loading: boolean) => void;
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => void;
  clearAuth: () => void;
}

// Standalone function to clear auth state - can be called outside React components
export function clearAuthState(): void {
  clearAuthCredentials();
  useAuthStore.getState().setUser(null);
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      isLoading: false,
      
      setUser: (user) => set({ user }),
      setLoading: (loading) => set({ isLoading: loading }),
      
      login: async (username: string, password: string) => {
        set({ isLoading: true });
        
        try {
          // Test authentication using the centralized API function
          const success = await testAuth(username, password);
          
          if (success) {
            const user: User = {
              username,
              isAuthenticated: true,
            };
            
            set({ user, isLoading: false });
            // Store credentials for future requests
            setAuthCredentials(username, password);
            return true;
          } else {
            set({ user: null, isLoading: false });
            return false;
          }
        } catch (error) {
          console.error("Login failed:", error);
          set({ user: null, isLoading: false });
          return false;
        }
      },
      
      logout: () => {
        set({ user: null });
        clearAuthCredentials();
      },
      
      clearAuth: () => {
        set({ user: null });
        clearAuthCredentials();
      },
    }),
    {
      name: "auth-storage",
      storage: createJSONStorage(() => localStorage),
      // Only persist user info, not loading state
      partialize: (state) => ({ user: state.user }),
    }
  )
);