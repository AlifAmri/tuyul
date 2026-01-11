import { AuthResponse, LoginRequest, RegisterRequest, User, UserStats } from '@/types/auth';
import apiClient from '../client';

export const authService = {
  login: async (data: LoginRequest): Promise<AuthResponse> => {
    return await apiClient.post('/api/v1/auth/login', data);
  },

  register: async (data: RegisterRequest): Promise<AuthResponse> => {
    return await apiClient.post('/api/v1/auth/register', data);
  },

  logout: async (): Promise<void> => {
    await apiClient.post('/api/v1/auth/logout');
  },

  getMe: async (): Promise<User> => {
    return await apiClient.get('/api/v1/auth/me');
  },

  getStats: async (): Promise<UserStats> => {
    return await apiClient.get('/api/v1/users/stats');
  },

  refreshToken: async (refreshToken: string): Promise<{ access_token: string }> => {
    return await apiClient.post('/api/v1/auth/refresh', {
      refresh_token: refreshToken,
    });
  },

  changePassword: async (oldPassword: string, newPassword: string): Promise<void> => {
    await apiClient.post('/api/v1/users/password', {
      old_password: oldPassword,
      new_password: newPassword,
    });
  },
};
