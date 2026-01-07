# Frontend Architecture

## Overview

TUYUL's frontend is a modern, responsive single-page application (SPA) built with React, TypeScript, Vite, TailwindCSS, and Radix UI. It provides a professional trading interface with real-time updates via WebSocket.

---

## Tech Stack

### Core
- **React 18+**: Component-based UI framework
- **TypeScript 5+**: Type-safe JavaScript
- **Vite**: Fast build tool and dev server
- **React Router v6**: Client-side routing

### Styling
- **TailwindCSS**: Utility-first CSS framework
- **Radix UI**: Unstyled, accessible component primitives
- **Custom theme**: Light/dark mode support

### State Management
- **Zustand**: Lightweight state management
- **React Query (TanStack Query)**: Server state management
- **Context API**: For theme and auth context

### Real-time Communication
- **WebSocket API**: Native WebSocket with auto-reconnect
- **Event-driven updates**: Real-time market data and order updates

### Form Handling
- **React Hook Form**: Performant form validation
- **Zod**: Schema validation

### Data Visualization
- **Recharts**: Charts for market data (optional)
- **Custom tables**: Sortable, filterable data tables

---

## Project Structure

```
frontend/
├── src/
│   ├── assets/                    # Static assets (images, icons)
│   │   ├── icons/
│   │   └── images/
│   ├── components/                # Reusable UI components
│   │   ├── ui/                    # Radix UI wrappers
│   │   │   ├── Button.tsx
│   │   │   ├── Dialog.tsx
│   │   │   ├── Input.tsx
│   │   │   ├── Select.tsx
│   │   │   ├── Table.tsx
│   │   │   ├── Toast.tsx
│   │   │   └── ...
│   │   ├── layout/                # Layout components
│   │   │   ├── Header.tsx
│   │   │   ├── Sidebar.tsx
│   │   │   ├── Footer.tsx
│   │   │   └── MainLayout.tsx
│   │   └── common/                # Common components
│   │       ├── ErrorBoundary.tsx
│   │       ├── Loading.tsx
│   │       ├── EmptyState.tsx
│   │       └── ProtectedRoute.tsx
│   ├── features/                  # Feature-specific components
│   │   ├── auth/
│   │   │   ├── LoginForm.tsx
│   │   │   ├── useAuth.ts
│   │   │   └── authSlice.ts
│   │   ├── dashboard/
│   │   │   ├── Dashboard.tsx
│   │   │   ├── StatsCard.tsx
│   │   │   └── RecentOrders.tsx
│   │   ├── market/
│   │   │   ├── MarketTable.tsx
│   │   │   ├── PumpScoreCard.tsx
│   │   │   ├── GapAnalysisCard.tsx
│   │   │   ├── MarketDetail.tsx
│   │   │   └── useMarketData.ts
│   │   ├── trading/
│   │   │   ├── TradeForm.tsx
│   │   │   ├── OrdersTable.tsx
│   │   │   ├── OrderDetail.tsx
│   │   │   ├── BalanceCard.tsx
│   │   │   └── useTrading.ts
│   │   └── admin/
│   │       ├── UserManagement.tsx
│   │       ├── UserForm.tsx
│   │       ├── SystemStats.tsx
│   │       └── useAdmin.ts
│   ├── hooks/                     # Custom React hooks
│   │   ├── useWebSocket.ts
│   │   ├── useAuth.ts
│   │   ├── useTheme.ts
│   │   ├── useDebounce.ts
│   │   └── usePagination.ts
│   ├── services/                  # API clients
│   │   ├── api.ts                 # Axios instance
│   │   ├── auth.service.ts
│   │   ├── user.service.ts
│   │   ├── market.service.ts
│   │   ├── trading.service.ts
│   │   └── websocket.service.ts
│   ├── stores/                    # Zustand stores
│   │   ├── authStore.ts
│   │   ├── marketStore.ts
│   │   ├── tradingStore.ts
│   │   └── themeStore.ts
│   ├── types/                     # TypeScript types
│   │   ├── api.types.ts
│   │   ├── user.types.ts
│   │   ├── market.types.ts
│   │   ├── order.types.ts
│   │   └── common.types.ts
│   ├── utils/                     # Utility functions
│   │   ├── format.ts              # Number, date formatting
│   │   ├── validation.ts          # Form validation helpers
│   │   ├── currency.ts            # Currency utilities
│   │   └── storage.ts             # LocalStorage helpers
│   ├── styles/                    # Global styles
│   │   ├── globals.css
│   │   └── theme.css
│   ├── pages/                     # Page components
│   │   ├── LoginPage.tsx
│   │   ├── DashboardPage.tsx
│   │   ├── MarketPage.tsx
│   │   ├── TradingPage.tsx
│   │   ├── SettingsPage.tsx
│   │   └── AdminPage.tsx
│   ├── App.tsx                    # Root component
│   ├── main.tsx                   # Entry point
│   └── vite-env.d.ts
├── public/
│   └── favicon.ico
├── index.html
├── package.json
├── tsconfig.json
├── tailwind.config.js
├── postcss.config.js
└── vite.config.ts
```

---

## Key Features & Components

### 1. Authentication

#### Login Page
```typescript
// src/pages/LoginPage.tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useAuth } from '@/features/auth/useAuth';

const loginSchema = z.object({
  username: z.string().min(3),
  password: z.string().min(8),
});

type LoginForm = z.infer<typeof loginSchema>;

export function LoginPage() {
  const { login, isLoading } = useAuth();
  const { register, handleSubmit, formState: { errors } } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
  });

  const onSubmit = async (data: LoginForm) => {
    await login(data.username, data.password);
  };

  return (
    <div className="min-h-screen flex items-center justify-center">
      <form onSubmit={handleSubmit(onSubmit)} className="w-full max-w-md">
        <Input {...register('username')} error={errors.username?.message} />
        <Input {...register('password')} type="password" error={errors.password?.message} />
        <Button type="submit" loading={isLoading}>Login</Button>
      </form>
    </div>
  );
}
```

#### Auth Store (Zustand)
```typescript
// src/stores/authStore.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface User {
  id: string;
  username: string;
  email: string;
  role: 'admin' | 'user';
}

interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  isAuthenticated: boolean;
  setAuth: (user: User, accessToken: string, refreshToken: string) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      accessToken: null,
      refreshToken: null,
      isAuthenticated: false,
      setAuth: (user, accessToken, refreshToken) =>
        set({ user, accessToken, refreshToken, isAuthenticated: true }),
      clearAuth: () =>
        set({ user: null, accessToken: null, refreshToken: null, isAuthenticated: false }),
    }),
    {
      name: 'auth-storage',
    }
  )
);
```

---

### 2. Market Analysis

#### Market Table
```typescript
// src/features/market/MarketTable.tsx
import { useQuery } from '@tanstack/react-query';
import { marketService } from '@/services/market.service';
import { Table } from '@/components/ui/Table';
import { useState } from 'react';

type SortKey = 'last_price' | 'change_24h' | 'volume_idr' | 'pump_score' | 'gap_percentage';

export function MarketTable() {
  const [sortBy, setSortBy] = useState<SortKey>('pump_score');
  const [filter, setFilter] = useState('');
  
  const { data, isLoading } = useQuery({
    queryKey: ['market', 'summary'],
    queryFn: () => marketService.getSummary(),
    refetchInterval: 5000, // Refresh every 5 seconds
  });

  const sortedData = data?.markets
    .filter(m => m.pair.includes(filter.toLowerCase()))
    .sort((a, b) => b[sortBy] - a[sortBy]);

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <Input 
          placeholder="Search pair..." 
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <Select value={sortBy} onValueChange={setSortBy}>
          <option value="pump_score">Pump Score</option>
          <option value="gap_percentage">Gap %</option>
          <option value="change_24h">Change 24h</option>
          <option value="volume_idr">Volume</option>
        </Select>
      </div>
      
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Pair</TableHead>
            <TableHead>Price</TableHead>
            <TableHead>Change 24h</TableHead>
            <TableHead>Volume</TableHead>
            <TableHead>Pump Score</TableHead>
            <TableHead>Gap %</TableHead>
            <TableHead>Action</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedData?.map((market) => (
            <TableRow key={market.pair}>
              <TableCell>{market.pair.toUpperCase()}</TableCell>
              <TableCell>{formatCurrency(market.last_price)}</TableCell>
              <TableCell className={market.change_24h > 0 ? 'text-green-600' : 'text-red-600'}>
                {market.change_24h > 0 ? '+' : ''}{market.change_24h.toFixed(2)}%
              </TableCell>
              <TableCell>{formatVolume(market.volume_idr)}</TableCell>
              <TableCell>
                <PumpScoreBadge score={market.pump_score} />
              </TableCell>
              <TableCell>{market.gap_percentage.toFixed(2)}%</TableCell>
              <TableCell>
                <Button size="sm" onClick={() => handleTrade(market.pair)}>
                  Trade
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
```

#### WebSocket Integration
```typescript
// src/hooks/useWebSocket.ts
import { useEffect, useRef, useState } from 'react';
import { useAuthStore } from '@/stores/authStore';

interface WebSocketMessage {
  type: string;
  data: any;
}

export function useWebSocket() {
  const [isConnected, setIsConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<WebSocketMessage | null>(null);
  const ws = useRef<WebSocket | null>(null);
  const { accessToken } = useAuthStore();
  const reconnectTimeout = useRef<number>();

  const connect = () => {
    if (!accessToken) return;

    const wsUrl = `${import.meta.env.VITE_WS_URL}?token=${accessToken}`;
    ws.current = new WebSocket(wsUrl);

    ws.current.onopen = () => {
      console.log('WebSocket connected');
      setIsConnected(true);
      
      // Subscribe to market updates
      ws.current?.send(JSON.stringify({
        action: 'subscribe',
        channel: 'market_summary',
        pairs: [],
      }));
    };

    ws.current.onmessage = (event) => {
      const message: WebSocketMessage = JSON.parse(event.data);
      setLastMessage(message);
    };

    ws.current.onclose = () => {
      console.log('WebSocket disconnected');
      setIsConnected(false);
      
      // Auto-reconnect after 3 seconds
      reconnectTimeout.current = window.setTimeout(() => {
        connect();
      }, 3000);
    };

    ws.current.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  };

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeout.current) {
        clearTimeout(reconnectTimeout.current);
      }
      ws.current?.close();
    };
  }, [accessToken]);

  const send = (message: any) => {
    if (ws.current?.readyState === WebSocket.OPEN) {
      ws.current.send(JSON.stringify(message));
    }
  };

  return { isConnected, lastMessage, send };
}
```

---

### 3. Trading

#### Trade Form
```typescript
// src/features/trading/TradeForm.tsx
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { tradingService } from '@/services/trading.service';

const tradeSchema = z.object({
  pair: z.string(),
  buying_price: z.number().positive(),
  volume_idr: z.number().min(10000, 'Minimum 10,000 IDR'),
  target_profit: z.number().min(0.1).max(1000),
  stop_loss: z.number().min(0.1).max(100),
}).refine((data) => data.stop_loss < data.target_profit, {
  message: 'Stop-loss must be less than target profit',
  path: ['stop_loss'],
});

type TradeForm = z.infer<typeof tradeSchema>;

interface TradeFormProps {
  defaultPair?: string;
  onSuccess?: () => void;
}

export function TradeForm({ defaultPair, onSuccess }: TradeFormProps) {
  const queryClient = useQueryClient();
  
  const { register, handleSubmit, formState: { errors }, watch } = useForm<TradeForm>({
    resolver: zodResolver(tradeSchema),
    defaultValues: {
      pair: defaultPair || 'btcidr',
      target_profit: 5.0,
      stop_loss: 3.0,
    },
  });

  const mutation = useMutation({
    mutationFn: tradingService.placeBuyOrder,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['orders'] });
      queryClient.invalidateQueries({ queryKey: ['balance'] });
      onSuccess?.();
    },
  });

  const onSubmit = (data: TradeForm) => {
    mutation.mutate(data);
  };

  const buyingPrice = watch('buying_price');
  const volumeIdr = watch('volume_idr');
  const amount = buyingPrice > 0 ? volumeIdr / buyingPrice : 0;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      <Select {...register('pair')}>
        <option value="btcidr">BTC/IDR</option>
        <option value="ethidr">ETH/IDR</option>
        {/* More pairs */}
      </Select>

      <Input
        {...register('buying_price', { valueAsNumber: true })}
        label="Buying Price (IDR)"
        type="number"
        error={errors.buying_price?.message}
      />

      <Input
        {...register('volume_idr', { valueAsNumber: true })}
        label="Volume (IDR)"
        type="number"
        error={errors.volume_idr?.message}
      />

      <div className="bg-gray-100 p-3 rounded">
        <p className="text-sm">Amount: {amount.toFixed(8)}</p>
      </div>

      <Input
        {...register('target_profit', { valueAsNumber: true })}
        label="Target Profit (%)"
        type="number"
        step="0.1"
        error={errors.target_profit?.message}
      />

      <Input
        {...register('stop_loss', { valueAsNumber: true })}
        label="Stop Loss (%)"
        type="number"
        step="0.1"
        error={errors.stop_loss?.message}
      />

      <div className="bg-blue-50 p-3 rounded">
        <p className="text-sm font-medium">Expected sell price:</p>
        <p className="text-lg">{(buyingPrice * (1 + watch('target_profit') / 100)).toLocaleString('id-ID')}</p>
      </div>

      <Button type="submit" loading={mutation.isPending} fullWidth>
        Place Buy Order
      </Button>

      {mutation.isError && (
        <Alert variant="error">
          {mutation.error.message}
        </Alert>
      )}
    </form>
  );
}
```

#### Orders Table
```typescript
// src/features/trading/OrdersTable.tsx
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { tradingService } from '@/services/trading.service';
import { Table } from '@/components/ui/Table';
import { Badge } from '@/components/ui/Badge';

export function OrdersTable() {
  const queryClient = useQueryClient();
  
  const { data, isLoading } = useQuery({
    queryKey: ['orders'],
    queryFn: () => tradingService.getOrders({ status: 'all', limit: 50 }),
  });

  const cancelMutation = useMutation({
    mutationFn: tradingService.cancelOrder,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['orders'] });
    },
  });

  const sellMutation = useMutation({
    mutationFn: tradingService.manualSell,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['orders'] });
    },
  });

  if (isLoading) return <Loading />;

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Pair</TableHead>
          <TableHead>Side</TableHead>
          <TableHead>Price</TableHead>
          <TableHead>Amount</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Target Profit</TableHead>
          <TableHead>Stop Loss</TableHead>
          <TableHead>Created</TableHead>
          <TableHead>Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {data?.orders.map((order) => (
          <TableRow key={order.id}>
            <TableCell>{order.pair.toUpperCase()}</TableCell>
            <TableCell>
              <Badge variant={order.side === 'buy' ? 'success' : 'warning'}>
                {order.side.toUpperCase()}
              </Badge>
            </TableCell>
            <TableCell>{formatCurrency(order.price)}</TableCell>
            <TableCell>{order.amount.toFixed(8)}</TableCell>
            <TableCell>
              <OrderStatusBadge status={order.status} />
            </TableCell>
            <TableCell>{order.target_profit}%</TableCell>
            <TableCell>{order.stop_loss}%</TableCell>
            <TableCell>{formatDate(order.created_at)}</TableCell>
            <TableCell>
              <div className="flex gap-2">
                {order.status === 'open' && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => cancelMutation.mutate(order.id)}
                    loading={cancelMutation.isPending}
                  >
                    Cancel
                  </Button>
                )}
                {order.status === 'filled' && order.side === 'buy' && (
                  <Button
                    size="sm"
                    variant="default"
                    onClick={() => sellMutation.mutate(order.id)}
                    loading={sellMutation.isPending}
                  >
                    Sell Now
                  </Button>
                )}
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
```

---

### 4. Admin Panel

#### User Management
```typescript
// src/features/admin/UserManagement.tsx
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { userService } from '@/services/user.service';
import { Table } from '@/components/ui/Table';
import { Dialog } from '@/components/ui/Dialog';
import { useState } from 'react';

export function UserManagement() {
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: () => userService.getAllUsers(),
  });

  const deleteMutation = useMutation({
    mutationFn: userService.deleteUser,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">User Management</h2>
        <Button onClick={() => setIsCreateDialogOpen(true)}>
          Create User
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Username</TableHead>
            <TableHead>Email</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>API Key</TableHead>
            <TableHead>Last Login</TableHead>
            <TableHead>Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {data?.users.map((user) => (
            <TableRow key={user.id}>
              <TableCell>{user.username}</TableCell>
              <TableCell>{user.email}</TableCell>
              <TableCell>
                <Badge variant={user.role === 'admin' ? 'default' : 'secondary'}>
                  {user.role}
                </Badge>
              </TableCell>
              <TableCell>
                <StatusBadge status={user.status} />
              </TableCell>
              <TableCell>
                {user.has_api_key ? '✓' : '✗'}
              </TableCell>
              <TableCell>{formatDate(user.last_login_at)}</TableCell>
              <TableCell>
                <DropdownMenu>
                  <DropdownMenuItem onClick={() => handleEdit(user)}>
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => handleResetPassword(user)}>
                    Reset Password
                  </DropdownMenuItem>
                  <DropdownMenuItem 
                    onClick={() => deleteMutation.mutate(user.id)}
                    className="text-red-600"
                  >
                    Delete
                  </DropdownMenuItem>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
        <UserForm onSuccess={() => setIsCreateDialogOpen(false)} />
      </Dialog>
    </div>
  );
}
```

---

## Styling & Theming

### TailwindCSS Configuration
```javascript
// tailwind.config.js
/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#f0f9ff',
          100: '#e0f2fe',
          500: '#0ea5e9',
          600: '#0284c7',
          700: '#0369a1',
        },
        // ... more colors
      },
    },
  },
  plugins: [
    require('@tailwindcss/forms'),
  ],
}
```

### Theme Provider
```typescript
// src/hooks/useTheme.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface ThemeState {
  theme: 'light' | 'dark';
  toggleTheme: () => void;
  setTheme: (theme: 'light' | 'dark') => void;
}

export const useTheme = create<ThemeState>()(
  persist(
    (set) => ({
      theme: 'light',
      toggleTheme: () =>
        set((state) => ({
          theme: state.theme === 'light' ? 'dark' : 'light',
        })),
      setTheme: (theme) => set({ theme }),
    }),
    {
      name: 'theme-storage',
    }
  )
);

// Apply theme to document
export function useThemeEffect() {
  const { theme } = useTheme();
  
  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark');
  }, [theme]);
}
```

---

## API Service Layer

### Axios Configuration
```typescript
// src/services/api.ts
import axios from 'axios';
import { useAuthStore } from '@/stores/authStore';

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor (add auth token)
api.interceptors.request.use(
  (config) => {
    const { accessToken } = useAuthStore.getState();
    if (accessToken) {
      config.headers.Authorization = `Bearer ${accessToken}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor (handle token refresh)
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;

    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;

      try {
        const { refreshToken } = useAuthStore.getState();
        const { data } = await axios.post(`${import.meta.env.VITE_API_URL}/auth/refresh`, {
          refresh_token: refreshToken,
        });

        const { setAuth } = useAuthStore.getState();
        setAuth(data.data.user, data.data.access_token, refreshToken);

        originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`;
        return api(originalRequest);
      } catch (refreshError) {
        // Refresh failed, logout user
        useAuthStore.getState().clearAuth();
        window.location.href = '/login';
        return Promise.reject(refreshError);
      }
    }

    return Promise.reject(error);
  }
);
```

---

## Performance Optimization

### Code Splitting
```typescript
// src/App.tsx
import { lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Loading } from '@/components/common/Loading';

const DashboardPage = lazy(() => import('@/pages/DashboardPage'));
const MarketPage = lazy(() => import('@/pages/MarketPage'));
const TradingPage = lazy(() => import('@/pages/TradingPage'));
const AdminPage = lazy(() => import('@/pages/AdminPage'));

export function App() {
  return (
    <BrowserRouter>
      <Suspense fallback={<Loading />}>
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/market" element={<MarketPage />} />
          <Route path="/trading" element={<TradingPage />} />
          <Route path="/admin" element={<AdminPage />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}
```

### React Query Configuration
```typescript
// src/main.tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </React.StrictMode>
);
```

---

## Responsive Design

### Mobile-First Approach
```typescript
// Example responsive table
<div className="overflow-x-auto">
  <Table className="min-w-full">
    {/* Desktop view */}
    <TableBody className="hidden md:table-row-group">
      {/* Full table rows */}
    </TableBody>
    
    {/* Mobile view */}
    <div className="md:hidden space-y-4">
      {data.map((item) => (
        <Card key={item.id}>
          <CardContent>
            {/* Stacked layout for mobile */}
          </CardContent>
        </Card>
      ))}
    </div>
  </Table>
</div>
```

---

## Error Handling

### Error Boundary
```typescript
// src/components/common/ErrorBoundary.tsx
import { Component, ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: any) {
    console.error('Error boundary caught:', error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback || (
        <div className="flex items-center justify-center min-h-screen">
          <div className="text-center">
            <h1 className="text-2xl font-bold text-red-600">Something went wrong</h1>
            <p className="text-gray-600 mt-2">{this.state.error?.message}</p>
            <Button onClick={() => window.location.reload()} className="mt-4">
              Reload Page
            </Button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
```

---

## Testing

### Component Testing (Vitest + React Testing Library)
```typescript
// src/features/trading/TradeForm.test.tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { TradeForm } from './TradeForm';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

describe('TradeForm', () => {
  const queryClient = new QueryClient();

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );

  it('renders form fields', () => {
    render(<TradeForm />, { wrapper });
    
    expect(screen.getByLabelText(/buying price/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/volume/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/target profit/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/stop loss/i)).toBeInTheDocument();
  });

  it('validates minimum volume', async () => {
    render(<TradeForm />, { wrapper });
    
    const volumeInput = screen.getByLabelText(/volume/i);
    fireEvent.change(volumeInput, { target: { value: '5000' } });
    fireEvent.submit(screen.getByRole('button', { name: /place buy order/i }));
    
    await waitFor(() => {
      expect(screen.getByText(/minimum 10,000 idr/i)).toBeInTheDocument();
    });
  });
});
```

---

## Build & Deployment

### Environment Variables
```bash
# .env.development
VITE_API_URL=http://localhost:8080/api/v1
VITE_WS_URL=ws://localhost:8081/ws

# .env.production
VITE_API_URL=https://api.tuyul.com/api/v1
VITE_WS_URL=wss://api.tuyul.com/ws
```

### Build Configuration
```typescript
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom', 'react-router-dom'],
          ui: ['@radix-ui/react-dialog', '@radix-ui/react-select'],
        },
      },
    },
  },
});
```

### Vercel Deployment

#### vercel.json
```json
{
  "buildCommand": "npm run build",
  "outputDirectory": "dist",
  "devCommand": "npm run dev",
  "installCommand": "npm install",
  "framework": "vite",
  "rewrites": [
    {
      "source": "/(.*)",
      "destination": "/index.html"
    }
  ],
  "headers": [
    {
      "source": "/assets/(.*)",
      "headers": [
        {
          "key": "Cache-Control",
          "value": "public, max-age=31536000, immutable"
        }
      ]
    }
  ],
  "env": {
    "VITE_API_URL": "@vite_api_url",
    "VITE_WS_URL": "@vite_ws_url"
  }
}
```

#### Environment Variables (Vercel)
```bash
# Production environment variables (set in Vercel Dashboard)
VITE_API_URL=https://api.tuyul.com/api/v1
VITE_WS_URL=wss://api.tuyul.com/ws

# Preview/Development
VITE_API_URL=https://api-staging.tuyul.com/api/v1
VITE_WS_URL=wss://api-staging.tuyul.com/ws
```

#### Deploy Commands
```bash
# Install Vercel CLI
npm i -g vercel

# Login to Vercel
vercel login

# Deploy to preview
vercel

# Deploy to production
vercel --prod

# Or use GitHub integration (recommended)
# Push to main branch → auto-deploy to production
# Push to other branches → auto-deploy to preview
```

#### Build Optimization for Vercel
```typescript
// vite.config.ts - optimized for Vercel
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom', 'react-router-dom'],
          ui: ['@radix-ui/react-dialog', '@radix-ui/react-select'],
          query: ['@tanstack/react-query'],
          state: ['zustand'],
        },
      },
    },
    chunkSizeWarningLimit: 1000,
  },
  server: {
    port: 5173,
    strictPort: false,
  },
});
```

#### GitHub Actions (Optional CI/CD)
```yaml
# .github/workflows/deploy.yml
name: Deploy to Vercel

on:
  push:
    branches:
      - main
      - develop

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '20'
          
      - name: Install dependencies
        run: npm ci
        
      - name: Run tests
        run: npm test
        
      - name: Build
        run: npm run build
        
      - name: Deploy to Vercel
        uses: amondnet/vercel-action@v20
        with:
          vercel-token: ${{ secrets.VERCEL_TOKEN }}
          vercel-org-id: ${{ secrets.VERCEL_ORG_ID }}
          vercel-project-id: ${{ secrets.VERCEL_PROJECT_ID }}
          vercel-args: ${{ github.ref == 'refs/heads/main' && '--prod' || '' }}
```

---

## Future Enhancements

- [ ] Progressive Web App (PWA) support
- [ ] Push notifications
- [ ] Advanced charting (TradingView integration)
- [ ] Export data to CSV/Excel
- [ ] Mobile app (React Native)
- [ ] Internationalization (i18n)
- [ ] Accessibility improvements (WCAG 2.1 AA)
- [ ] Performance monitoring (Sentry, LogRocket)

