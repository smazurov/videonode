import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

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

// Basic Auth API functions
const API_BASE_URL = "http://localhost:8090";

async function makeAuthenticatedRequest(url: string, username: string, password: string) {
  const credentials = btoa(`${username}:${password}`);
  
  try {
    return await fetch(url, {
      headers: {
        'Authorization': `Basic ${credentials}`,
        'Content-Type': 'application/json',
      },
    });
  } catch (error) {
    console.error("API request failed:", error);
    throw error;
  }
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
          // Test authentication with a protected endpoint (devices requires auth)
          const response = await makeAuthenticatedRequest(
            `${API_BASE_URL}/api/devices`,
            username,
            password
          );
          
          if (response.ok) {
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