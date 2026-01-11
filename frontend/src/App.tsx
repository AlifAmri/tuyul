import { useEffect, lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAuthStore } from './stores/authStore';
import { useThemeStore } from './stores/themeStore';
import { authService } from './api/services/auth';
import { ProtectedRoute } from './components/common/ProtectedRoute';
import { Layout } from './components/layout/Layout';
import { Toaster } from './components/common/Toaster';
import { LoadingSpinner } from './components/common/LoadingSpinner';
import { WebSocketProvider } from './contexts/WebSocketContext';
import { useNotifications } from './hooks/useNotifications';

// Lazy load pages for code splitting
const LoginPage = lazy(() => import('./pages/LoginPage').then(m => ({ default: m.LoginPage })));
const RegisterPage = lazy(() => import('./pages/RegisterPage').then(m => ({ default: m.RegisterPage })));
const DashboardPage = lazy(() => import('./pages/DashboardPage').then(m => ({ default: m.DashboardPage })));
const AllMarketsPage = lazy(() => import('./pages/AllMarketsPage').then(m => ({ default: m.default })));
const PumpScoresPage = lazy(() => import('./pages/PumpScoresPage').then(m => ({ default: m.default })));
const GapsPage = lazy(() => import('./pages/GapsPage').then(m => ({ default: m.default })));
const BotsPage = lazy(() => import('./pages/BotsPage').then(m => ({ default: m.BotsPage })));
const CopilotPage = lazy(() => import('./pages/CopilotPage').then(m => ({ default: m.CopilotPage })));
const SettingsPage = lazy(() => import('./pages/SettingsPage').then(m => ({ default: m.SettingsPage })));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

function AppContent() {
  const { isAuthenticated } = useAuthStore();
  
  // Always call useNotifications (hooks must be called unconditionally)
  // It will only be active when user is authenticated
  useNotifications();

  return (
    <>
      <Toaster />
      <Suspense fallback={
        <div className="min-h-screen bg-black flex items-center justify-center">
          <LoadingSpinner />
        </div>
      }>
        <Routes>
          {/* Public routes */}
          <Route
            path="/login"
            element={isAuthenticated ? <Navigate to="/dashboard" replace /> : <LoginPage />}
          />
          <Route
            path="/register"
            element={isAuthenticated ? <Navigate to="/dashboard" replace /> : <RegisterPage />}
          />

          {/* Protected routes */}
          <Route
            path="/dashboard"
            element={
              <ProtectedRoute>
                <Layout>
                  <DashboardPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/market"
            element={
              <ProtectedRoute>
                <Layout>
                  <AllMarketsPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/market/pumps"
            element={
              <ProtectedRoute>
                <Layout>
                  <PumpScoresPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/market/gaps"
            element={
              <ProtectedRoute>
                <Layout>
                  <GapsPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/bots"
            element={
              <ProtectedRoute>
                <Layout>
                  <BotsPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/copilot"
            element={
              <ProtectedRoute>
                <Layout>
                  <CopilotPage />
                </Layout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/settings/*"
            element={
              <ProtectedRoute>
                <Layout>
                  <SettingsPage />
                </Layout>
              </ProtectedRoute>
            }
          />

          {/* Default redirect */}
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </Suspense>
    </>
  );
}

function App() {
  const { initAuth, isAuthChecking, setUser, logout, setAuthChecking } = useAuthStore();
  const { initTheme } = useThemeStore();

  useEffect(() => {
    initTheme();
    
    // Initialize auth from localStorage
    initAuth();
    
    // Validate token with backend if token exists
    const token = localStorage.getItem('tuyul_access_token');
    if (token) {
      setAuthChecking(true);
      authService
        .getMe()
        .then((user) => {
          // Token is valid, update user data
          setUser(user);
          localStorage.setItem('tuyul_user', JSON.stringify(user));
        })
        .catch((error) => {
          // Token is invalid or expired
          console.error('[Auth] Token validation failed:', error);
          logout();
          // Redirect to login will happen automatically via routing
        });
    }
  }, [initAuth, initTheme, setUser, logout, setAuthChecking]);

  // Show loading spinner while checking authentication
  if (isAuthChecking) {
    return (
      <div className="min-h-screen bg-black flex items-center justify-center">
        <LoadingSpinner />
      </div>
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <WebSocketProvider>
        <BrowserRouter>
          <AppContent />
        </BrowserRouter>
      </WebSocketProvider>
    </QueryClientProvider>
  );
}

export default App;

