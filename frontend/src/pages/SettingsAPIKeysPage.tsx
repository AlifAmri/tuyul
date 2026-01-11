import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { apiKeyService } from '@/api/services/apiKey';
import { authService } from '@/api/services/auth';
import { useAuthStore } from '@/stores/authStore';
import { APIKeyRequest } from '@/types/apiKey';
import { formatRelativeTime } from '@/utils/formatters';
import { cn } from '@/utils/cn';
import { AlertModal } from '@/components/common/AlertModal';

export default function SettingsAPIKeysPage() {
  const queryClient = useQueryClient();
  const { user, setUser } = useAuthStore();
  const apiKey = user?.api_key;
  const [showForm, setShowForm] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [formData, setFormData] = useState<APIKeyRequest>({
    api_key: '',
    api_secret: '',
    label: '',
  });
  const [testingKey, setTestingKey] = useState(false);
  const [formError, setFormError] = useState<string>('');
  const [alertModal, setAlertModal] = useState<{
    open: boolean;
    title: string;
    message: string;
    type: 'success' | 'error' | 'info' | 'warning';
  }>({
    open: false,
    title: '',
    message: '',
    type: 'info',
  });

  // Create or update API key mutation
  const saveKeyMutation = useMutation({
    mutationFn: (data: APIKeyRequest) => apiKeyService.createAPIKey(data),
    onSuccess: async () => {
      // Refresh user data to get updated API key
      const updatedUser = await authService.getMe();
      setUser(updatedUser);
      queryClient.invalidateQueries({ queryKey: ['auth', 'me'] });
      setShowForm(false);
      setIsEditing(false);
      resetForm();
      setFormError('');
    },
    onError: (error: any) => {
      // Extract error message from API response
      const errorMessage = error?.response?.data?.error?.message || error?.message || 'Failed to save API key. Please try again.';
      setFormError(errorMessage);
    },
  });

  // Delete API key mutation
  const deleteKeyMutation = useMutation({
    mutationFn: () => apiKeyService.deleteAPIKey(),
    onSuccess: async () => {
      // Refresh user data to remove API key
      const updatedUser = await authService.getMe();
      setUser(updatedUser);
      queryClient.invalidateQueries({ queryKey: ['auth', 'me'] });
    },
  });

  // Test API key (get account info)
  const testKeyMutation = useMutation({
    mutationFn: () => apiKeyService.getAccountInfo(),
    onSuccess: (data: any) => {
      const balance = parseFloat(data.balance.idr || '0').toLocaleString();
      setAlertModal({
        open: true,
        title: 'API Key is Valid',
        message: `Your API key is working correctly!\n\nIDR Balance: Rp ${balance}`,
        type: 'success',
      });
      setTestingKey(false);
    },
    onError: () => {
      setAlertModal({
        open: true,
        title: 'API Key Validation Failed',
        message: 'Failed to connect with this API key. Please check your credentials and try again.',
        type: 'error',
      });
      setTestingKey(false);
    },
  });

  const resetForm = () => {
    setFormData({
      api_key: '',
      api_secret: '',
      label: '',
    });
    setFormError('');
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    saveKeyMutation.mutate(formData);
  };

  const handleEdit = () => {
    // Pre-fill form with current values (though we can't show the actual key/secret)
    setFormData({
      api_key: '',
      api_secret: '',
      label: '',
    });
    setIsEditing(true);
    setShowForm(true);
  };

  const handleTest = () => {
    setTestingKey(true);
    testKeyMutation.mutate();
  };

  const handleDelete = () => {
    if (confirm('Are you sure you want to delete your API key? This will disable live trading.')) {
      deleteKeyMutation.mutate();
    }
  };

  return (
    <div className="max-w-4xl mx-auto">
      <div className="space-y-6">
        {/* API Key Card */}
        {!apiKey ? (
          <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-12 overflow-hidden border border-gray-800">
            <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
            <div className="relative z-10 flex flex-col items-center justify-center text-center">
              <div className="flex items-center justify-center mb-6">
                <img 
                  src="/tuyul-hope.png" 
                  alt="Hopeful Tuyul" 
                  className="w-64 h-auto"
                />
              </div>
              <h3 className="text-2xl font-bold text-white mb-6">
                I Need Your Key to Start Working!
              </h3>
              <button
                onClick={() => {
                  setIsEditing(false);
                  setShowForm(true);
                }}
                className="px-6 py-3 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors"
              >
                Give Me the Key
              </button>
            </div>
          </div>
        ) : (
          <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-12 overflow-hidden border border-gray-800">
            <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
            <div className="relative z-10 flex flex-col items-center justify-center text-center">
              <div className="flex items-center justify-center mb-6">
                <img 
                  src="/tuyul-letsgo.png" 
                  alt="Tuyul Ready" 
                  className="w-64 h-auto"
                />
              </div>
              <h3 className="text-2xl font-bold text-white mb-6">
                I Got the Key! Ready to Steal!
              </h3>
              <div className="flex gap-2 mb-4">
                <button
                  onClick={handleTest}
                  disabled={testingKey}
                  className="px-4 py-2 bg-gray-700 hover:bg-gray-600 disabled:bg-gray-800 disabled:cursor-not-allowed text-white text-sm rounded-lg transition-colors"
                >
                  {testingKey ? 'Testing...' : 'Test'}
                </button>
                <button
                  onClick={handleEdit}
                  className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white text-sm rounded-lg transition-colors"
                >
                  Edit
                </button>
                <button
                  onClick={handleDelete}
                  disabled={deleteKeyMutation.isPending}
                  className="px-4 py-2 bg-red-600 hover:bg-red-700 disabled:bg-gray-800 disabled:cursor-not-allowed text-white text-sm rounded-lg transition-colors"
                >
                  {deleteKeyMutation.isPending ? 'Deleting...' : 'Delete'}
                </button>
              </div>
              <div className="flex items-center justify-center mb-4">
                <span className={cn(
                  'px-3 py-1 rounded-full text-xs font-medium',
                  apiKey.is_valid
                    ? 'text-green-500 bg-green-500/10'
                    : 'text-red-500 bg-red-500/10'
                )}>
                  {apiKey.is_valid ? 'Valid' : 'Invalid'}
                </span>
              </div>
              <div className="space-y-1 text-sm">
                {apiKey.last_validated_at && (
                  <p className="text-gray-500 text-xs">
                    Last validated {formatRelativeTime(apiKey.last_validated_at)}
                  </p>
                )}
                <p className="text-gray-500 text-xs">
                  Created {formatRelativeTime(apiKey.created_at)}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Create/Edit API Key Form Modal */}
        {showForm && (
          <div
            className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4"
            onClick={() => {
              setShowForm(false);
              setIsEditing(false);
              resetForm();
            }}
          >
            <div
              className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-8 max-w-md w-full border border-gray-800"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
              <div className="relative z-10">
                <div className="flex items-center justify-between mb-6">
                  <h2 className="text-2xl font-bold text-white">
                    {isEditing ? 'Edit API Key' : 'Create API Key'}
                  </h2>
                  <button
                    onClick={() => {
                      setShowForm(false);
                      setIsEditing(false);
                      resetForm();
                    }}
                    className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
                  >
                    <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                {/* Error Message */}
                {formError && (
                  <div className="mb-4 p-4 bg-red-900/50 border border-red-800 rounded-lg">
                    <p className="text-red-400 text-sm">{formError}</p>
                  </div>
                )}

                <form onSubmit={handleSubmit} className="space-y-4">
                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">API Key</label>
                    <input
                      type="text"
                      placeholder={isEditing ? 'Enter new API key' : 'Your Indodax API Key'}
                      value={formData.api_key}
                      onChange={(e) => setFormData({ ...formData, api_key: e.target.value })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 font-mono"
                      required
                    />
                    {isEditing && (
                      <p className="text-gray-500 text-xs mt-1">
                        Enter a new API key to replace the existing one
                      </p>
                    )}
                  </div>

                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">API Secret</label>
                    <input
                      type="password"
                      placeholder={isEditing ? 'Enter new API secret' : 'Your Indodax API Secret'}
                      value={formData.api_secret}
                      onChange={(e) => setFormData({ ...formData, api_secret: e.target.value })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 font-mono"
                      required
                    />
                    {isEditing && (
                      <p className="text-gray-500 text-xs mt-1">
                        Enter a new API secret to replace the existing one
                      </p>
                    )}
                  </div>

                  <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                    <p className="text-gray-400 text-xs">
                      Get your API keys from Indodax Settings → API Management. Make sure to enable trading permissions.
                    </p>
                  </div>

                  <button
                    type="submit"
                    disabled={saveKeyMutation.isPending}
                    className="w-full px-4 py-3 bg-primary-600 hover:bg-primary-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
                  >
                    {saveKeyMutation.isPending ? (isEditing ? 'Updating...' : 'Creating...') : (isEditing ? 'Update API Key' : 'Create API Key')}
                  </button>
                </form>
              </div>
            </div>
          </div>
        )}

        {/* Alert Modal */}
        <AlertModal
          open={alertModal.open}
          onClose={() => setAlertModal({ ...alertModal, open: false })}
          title={alertModal.title}
          message={alertModal.message}
          type={alertModal.type}
        />
      </div>
    </div>
  );
}
