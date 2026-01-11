import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { useAuthStore } from '@/stores/authStore';
import { authService } from '@/api/services/auth';

export default function SettingsProfilePage() {
  const { user, logout } = useAuthStore();
  
  const [passwordData, setPasswordData] = useState({
    old_password: '',
    new_password: '',
    confirm_password: '',
  });
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  // Change password mutation
  const changePasswordMutation = useMutation({
    mutationFn: (data: { old_password: string; new_password: string }) => 
      authService.changePassword(data.old_password, data.new_password),
    onSuccess: () => {
      setSuccess('Password changed successfully!');
      setError('');
      setPasswordData({ old_password: '', new_password: '', confirm_password: '' });
      setTimeout(() => setSuccess(''), 3000);
    },
    onError: (error: any) => {
      setError(error.response?.data?.error?.message || 'Failed to change password');
      setSuccess('');
    },
  });

  const handlePasswordSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess('');

    if (passwordData.new_password !== passwordData.confirm_password) {
      setError('New passwords do not match');
      return;
    }

    if (passwordData.new_password.length < 8) {
      setError('New password must be at least 8 characters');
      return;
    }

    changePasswordMutation.mutate({
      old_password: passwordData.old_password,
      new_password: passwordData.new_password,
    });
  };

  const handleLogout = () => {
    logout();
    window.location.href = '/login';
  };

  return (
    <div className="max-w-4xl mx-auto">
      <div className="space-y-6">
        {/* Header */}
        <div>
          <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Profile & Settings</h1>
          <p className="text-gray-500 dark:text-gray-400 mt-1">
            Manage your account settings and preferences
          </p>
        </div>

        {/* Profile Information */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <h2 className="text-xl font-bold text-white mb-4">Profile Information</h2>
            <div className="space-y-4">
              <div>
                <label className="block text-gray-400 text-sm font-medium mb-2">Username</label>
                <div className="px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white">
                  {user?.username}
                </div>
              </div>
              <div>
                <label className="block text-gray-400 text-sm font-medium mb-2">Email</label>
                <div className="px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white">
                  {user?.email}
                </div>
              </div>
              <div>
                <label className="block text-gray-400 text-sm font-medium mb-2">Role</label>
                <div className="px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white capitalize">
                  {user?.role}
                </div>
              </div>
              {user?.status && (
                <div>
                  <label className="block text-gray-400 text-sm font-medium mb-2">Account Status</label>
                  <div className="px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white capitalize">
                    {user.status}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Change Password */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <h2 className="text-xl font-bold text-white mb-4">Change Password</h2>
            
            {error && (
              <div className="mb-4 px-4 py-3 bg-red-500/10 border border-red-500/50 rounded-lg text-red-500 text-sm">
                {error}
              </div>
            )}
            
            {success && (
              <div className="mb-4 px-4 py-3 bg-green-500/10 border border-green-500/50 rounded-lg text-green-500 text-sm">
                {success}
              </div>
            )}

            <form onSubmit={handlePasswordSubmit} className="space-y-4">
              <div>
                <label className="block text-gray-400 text-sm font-medium mb-2">Current Password</label>
                <input
                  type="password"
                  value={passwordData.old_password}
                  onChange={(e) => setPasswordData({ ...passwordData, old_password: e.target.value })}
                  className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                  required
                />
              </div>

              <div>
                <label className="block text-gray-400 text-sm font-medium mb-2">New Password</label>
                <input
                  type="password"
                  value={passwordData.new_password}
                  onChange={(e) => setPasswordData({ ...passwordData, new_password: e.target.value })}
                  className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                  required
                  minLength={8}
                />
                <p className="text-gray-500 text-xs mt-1">Must be at least 8 characters</p>
              </div>

              <div>
                <label className="block text-gray-400 text-sm font-medium mb-2">Confirm New Password</label>
                <input
                  type="password"
                  value={passwordData.confirm_password}
                  onChange={(e) => setPasswordData({ ...passwordData, confirm_password: e.target.value })}
                  className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                  required
                  minLength={8}
                />
              </div>

              <button
                type="submit"
                disabled={changePasswordMutation.isPending}
                className="px-6 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
              >
                {changePasswordMutation.isPending ? 'Changing...' : 'Change Password'}
              </button>
            </form>
          </div>
        </div>

        {/* Danger Zone */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-red-800">
          <div className="absolute inset-0 bg-gradient-to-br from-red-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <h2 className="text-xl font-bold text-white mb-4">Danger Zone</h2>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-white font-medium">Logout from all devices</p>
                <p className="text-gray-400 text-sm">This will log you out from all active sessions</p>
              </div>
              <button
                onClick={handleLogout}
                className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white font-medium rounded-lg transition-colors"
              >
                Logout
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

