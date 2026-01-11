import { APIKeyResponse } from './apiKey';

export interface User {
  id: string;
  username: string;
  email: string;
  role: 'admin' | 'user';
  status: 'active' | 'inactive' | 'suspended';
  has_api_key: boolean;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
  api_key?: APIKeyResponse | null;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface RegisterRequest {
  username: string;
  email: string;
  password: string;
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  user: User;
}

export interface RefreshTokenResponse {
  access_token: string;
  expires_in: number;
}
export interface UserStats {
  active_bots: number;
  total_profit_idr: number;
  avg_win_rate: number;
  total_trades: number;
}
