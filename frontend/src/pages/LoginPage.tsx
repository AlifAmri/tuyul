import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { authService } from '@/api/services/auth';
import { useAuthStore } from '@/stores/authStore';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

export function LoginPage() {
  const navigate = useNavigate();
  const { setUser, setTokens } = useAuthStore();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');

  const loginMutation = useMutation({
    mutationFn: authService.login,
    onSuccess: async (data) => {
      // Set tokens first
      setTokens(data.access_token, data.refresh_token);
      
      // Fetch complete user data including API key info
      try {
        const userData = await authService.getMe();
        setUser(userData);
      } catch (error) {
        // If getMe fails, fall back to user from login response
        console.error('Failed to fetch user data:', error);
        setUser(data.user);
      }
      
      navigate('/dashboard');
    },
    onError: (error: unknown) => {
      console.error('Login error:', error);
      const err = error as { 
        response?: { 
          data?: { 
            success?: boolean;
            error?: { 
              code?: string; 
              message?: string; 
              details?: string;
            } 
          } 
        };
        message?: string;
      };
      
      // Extract error message from the response
      let errorMessage = 'Login failed. Please try again.';
      
      if (err.response?.data?.error?.message) {
        errorMessage = err.response.data.error.message;
      } else if (err.message) {
        errorMessage = err.message;
      }
      
      setError(String(errorMessage));
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!username || !password) {
      setError('Please enter both username and password');
      return;
    }

    loginMutation.mutate({ username, password });
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-black px-4 py-12">
      <div className="w-full max-w-md">
        {/* Logo */}
        <div className="text-center mb-10">
          <div className="relative inline-block">
            <div className="absolute inset-0 bg-gradient-to-r from-amber-400/60 via-yellow-500/60 to-amber-400/60 rounded-lg blur-2xl animate-pulse"></div>
            <img 
              src="/logo.png" 
              alt="TUYUL" 
              className="relative h-64 w-auto mx-auto mb-4 animate-bounce-slow object-contain drop-shadow-[0_0_30px_rgba(251,191,36,0.8)]" 
            />
          </div>
          <p className="text-gray-400 animate-fade-in-up">Tuyul At Your Service</p>
        </div>

        {/* Login Form */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-3xl p-8 overflow-hidden border border-gray-800 backdrop-blur-xl">
          {/* Inner gradient overlay */}
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-blue-500/5 rounded-3xl pointer-events-none" />
          <div className="relative z-10">
          <h2 className="text-2xl font-bold text-white mb-6">Sign In</h2>

          {error && (
            <div className="mb-4 p-3 bg-red-900/30 border border-red-800 rounded-lg text-red-300 text-sm">
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-6">
            <div>
              <label htmlFor="username" className="block text-sm font-medium text-gray-300 mb-2">
                Username
              </label>
              <input
                id="username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                placeholder="Enter your username"
                disabled={loginMutation.isPending}
              />
            </div>

            <div>
              <label htmlFor="password" className="block text-sm font-medium text-gray-300 mb-2">
                Password
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full px-4 py-3 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                placeholder="Enter your password"
                disabled={loginMutation.isPending}
              />
            </div>

            <div className="pt-4">
              <button
                type="submit"
                disabled={loginMutation.isPending}
                className="w-full py-3 bg-primary-600 hover:bg-primary-700 text-white font-medium rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
              >
                {loginMutation.isPending ? (
                  <>
                    <LoadingSpinner size="sm" />
                    <span>Signing in...</span>
                  </>
                ) : (
                  'Sign In'
                )}
              </button>
            </div>
          </form>
          </div>
        </div>

        {/* Footer */}
        <p className="text-center text-gray-500 text-sm mt-8">
          © 2024 Enigma Venture. All rights reserved.
        </p>
      </div>
    </div>
  );
}

