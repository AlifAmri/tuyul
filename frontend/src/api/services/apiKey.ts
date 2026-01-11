import { AccountInfo, APIKey, APIKeyRequest, ValidateAPIKeyResponse } from '@/types/apiKey';
import apiClient from '../client';

export const apiKeyService = {
  // Get current user's API key status
  getAPIKey: async (): Promise<APIKey> => {
    return await apiClient.get('/api/v1/api-keys');
  },

  // Create and validate new API key
  createAPIKey: async (data: APIKeyRequest): Promise<APIKey> => {
    return await apiClient.post('/api/v1/api-keys', data);
  },

  // Delete current user's API key
  deleteAPIKey: async (): Promise<void> => {
    await apiClient.delete('/api/v1/api-keys');
  },

  // Validate API key with Indodax
  validateAPIKey: async (): Promise<ValidateAPIKeyResponse> => {
    return await apiClient.post('/api/v1/api-keys/validate');
  },

  // Get Indodax account info (balance)
  getAccountInfo: async (): Promise<AccountInfo> => {
    return await apiClient.get('/api/v1/api-keys/account-info');
  },
};
