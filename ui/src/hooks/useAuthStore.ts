import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { testAuth } from "../lib/api";

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
            localStorage.setItem('auth_credentials', btoa(`${username}:${password}`));
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
        localStorage.removeItem('auth_credentials');
      },
      
      clearAuth: () => {
        set({ user: null });
        localStorage.removeItem('auth_credentials');
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