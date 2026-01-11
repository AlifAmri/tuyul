export interface APIKeyResponse {
  user_id: string;
  is_valid: boolean;
  last_validated_at?: string;
  created_at: string;
  updated_at: string;
}

// Legacy interface for backward compatibility (if needed)
export interface APIKey {
  id: string;
  user_id: string;
  label: string;
  is_active: boolean;
  last_used_at?: string;
  created_at: string;
}

export interface APIKeyRequest {
  api_key: string;
  api_secret: string;
  label?: string;
}

export interface ValidateAPIKeyResponse {
  is_valid: boolean;
  message?: string;
}

export interface AccountInfo {
  balance: Record<string, string>;
  balance_hold: Record<string, string>;
}
