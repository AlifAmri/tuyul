import { create } from 'zustand';
import { User } from '@/types/auth';

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isAuthChecking: boolean;
  setUser: (user: User | null) => void;
  setTokens: (accessToken: string, refreshToken: string) => void;
  logout: () => void;
  initAuth: () => void;
  setAuthChecking: (checking: boolean) => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isAuthChecking: true,

  setUser: (user) => {
    if (user) {
      localStorage.setItem('tuyul_user', JSON.stringify(user));
    } else {
      localStorage.removeItem('tuyul_user');
    }
    set({ user, isAuthenticated: !!user, isAuthChecking: false });
  },

  setTokens: (accessToken, refreshToken) => {
    localStorage.setItem('tuyul_access_token', accessToken);
    localStorage.setItem('tuyul_refresh_token', refreshToken);
  },

  logout: () => {
    localStorage.removeItem('tuyul_access_token');
    localStorage.removeItem('tuyul_refresh_token');
    localStorage.removeItem('tuyul_user');
    set({ user: null, isAuthenticated: false, isAuthChecking: false });
  },

  initAuth: () => {
    const token = localStorage.getItem('tuyul_access_token');
    const userStr = localStorage.getItem('tuyul_user');
    
    if (token && userStr) {
      try {
        const user = JSON.parse(userStr);
        set({ user, isAuthenticated: true, isAuthChecking: false });
      } catch {
        // Invalid user data, clear storage
        localStorage.removeItem('tuyul_access_token');
        localStorage.removeItem('tuyul_refresh_token');
        localStorage.removeItem('tuyul_user');
        set({ isAuthChecking: false });
      }
    } else {
      set({ isAuthChecking: false });
    }
  },

  setAuthChecking: (checking) => set({ isAuthChecking: checking }),
}));

