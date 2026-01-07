# TUYUL Frontend Development Task List

## Project Overview
Frontend development for TUYUL crypto trading bot platform using React, TypeScript, Vite, TailwindCSS, and Radix UI.

---

## Phase 1: Project Setup & Configuration
**Estimated Time: 1-2 days**

### 1.1 Initial Setup
- [ ] Initialize Vite project with React + TypeScript template
- [ ] Setup project folder structure (components, features, hooks, services, stores, etc.)
- [ ] Configure path aliases (@/ for src/)
- [ ] Setup ESLint and Prettier
- [ ] Configure TypeScript (tsconfig.json)
- [ ] Create .gitignore for frontend

**Time: 3-4 hours**

### 1.2 Dependencies Installation
- [ ] Install core dependencies (React Router, React Query, Zustand)
- [ ] Install TailwindCSS and PostCSS
- [ ] Install Radix UI components (@radix-ui/react-*)
- [ ] Install form libraries (React Hook Form, Zod)
- [ ] Install utility libraries (axios, clsx, date-fns)
- [ ] Install dev dependencies (Vitest, Testing Library)

**Time: 2-3 hours**

### 1.3 TailwindCSS Configuration
- [ ] Configure tailwind.config.js (colors, fonts, spacing)
- [ ] Setup PostCSS configuration
- [ ] Create global styles (globals.css)
- [ ] Define theme colors (primary, success, warning, danger)
- [ ] Configure dark mode support
- [ ] Install @tailwindcss/forms plugin

**Time: 3-4 hours**

---

## Phase 2: Core Infrastructure
**Estimated Time: 2-3 days**

### 2.1 Routing Setup
- [ ] Configure React Router v6
- [ ] Create route structure
- [ ] Implement protected routes (ProtectedRoute component)
- [ ] Setup route guards (auth required, admin only)
- [ ] Implement 404 Not Found page
- [ ] Add route-based code splitting (lazy loading)

**Time: 4-5 hours**

### 2.2 API Service Layer
- [ ] Create Axios instance with base configuration
- [ ] Implement request interceptor (add auth token)
- [ ] Implement response interceptor (handle 401, token refresh)
- [ ] Create auth service (login, register, refresh, logout)
- [ ] Create user service (profile, update, list users)
- [ ] Create market service (summary, top pumps, pair details)
- [ ] Create trading service (place order, cancel, sell, balance)
- [ ] Error handling and response normalization

**Time: 8-10 hours**

### 2.3 WebSocket Service
- [ ] Create WebSocket connection manager
- [ ] Implement auto-connect on authentication
- [ ] Implement auto-reconnect with exponential backoff
- [ ] Implement subscribe/unsubscribe methods
- [ ] Handle incoming message routing by type
- [ ] Heartbeat/ping-pong implementation
- [ ] Connection state management

**Time: 6-8 hours**

### 2.4 State Management (Zustand)
- [ ] Create auth store (user, tokens, login/logout)
- [ ] Create theme store (light/dark mode toggle)
- [ ] Create market store (market data, real-time updates)
- [ ] Create trading store (orders, balance)
- [ ] Implement persistence (localStorage) for auth and theme
- [ ] Create store hooks for easy access

**Time: 6-8 hours**

---

## Phase 3: UI Component Library (Radix UI Wrappers)
**Estimated Time: 3-4 days**

### 3.1 Basic Components
- [ ] Button component (variants: default, outline, ghost, danger)
- [ ] Input component (with error states, labels)
- [ ] Textarea component
- [ ] Label component
- [ ] Badge component (variants: default, success, warning, danger)
- [ ] Card component (Card, CardHeader, CardContent, CardFooter)
- [ ] Loading spinner component

**Time: 6-8 hours**

### 3.2 Form Components
- [ ] Select component (Radix Select wrapper)
- [ ] Checkbox component
- [ ] Radio group component
- [ ] Switch component
- [ ] Form field wrapper (with label, error, helper text)
- [ ] Form validation error display

**Time: 4-6 hours**

### 3.3 Overlay Components
- [ ] Dialog/Modal component (Radix Dialog)
- [ ] Alert Dialog component (for confirmations)
- [ ] Popover component
- [ ] Tooltip component
- [ ] Dropdown Menu component
- [ ] Toast/Notification component

**Time: 6-8 hours**

### 3.4 Data Display Components
- [ ] Table component (Table, TableHeader, TableBody, TableRow, TableCell)
- [ ] Sortable table headers
- [ ] Pagination component
- [ ] Empty state component
- [ ] Status badge component
- [ ] Progress bar component

**Time: 6-8 hours**

### 3.5 Layout Components
- [ ] Header component (logo, navigation, user menu)
- [ ] Sidebar component (navigation links, collapsible)
- [ ] Footer component
- [ ] MainLayout component (header + sidebar + content)
- [ ] Mobile responsive navigation (hamburger menu)
- [ ] Theme toggle button (light/dark)

**Time: 6-8 hours**

---

## Phase 4: Authentication Feature
**Estimated Time: 2-3 days**

### 4.1 Auth Pages
- [ ] Login page with form
- [ ] Register page with form
- [ ] Forgot password page (UI only, if needed)
- [ ] Auth page layout (centered, branded)

**Time: 4-6 hours**

### 4.2 Auth Forms
- [ ] Login form with React Hook Form + Zod validation
- [ ] Register form with validation
- [ ] Form error handling and display
- [ ] Loading states during submission
- [ ] Success/error toast notifications

**Time: 4-6 hours**

### 4.3 Auth Logic
- [ ] useAuth hook (login, logout, register)
- [ ] Auto-redirect after login (to dashboard)
- [ ] Auto-redirect to login when unauthenticated
- [ ] Token refresh logic
- [ ] Logout functionality (clear tokens, redirect)

**Time: 4-6 hours**

### 4.4 Protected Routes
- [ ] ProtectedRoute component (check auth)
- [ ] Admin-only route wrapper
- [ ] Redirect logic for unauthenticated users
- [ ] Loading state while checking auth

**Time: 2-3 hours**

---

## Phase 5: Dashboard Page
**Estimated Time: 2-3 days**

### 5.1 Dashboard Layout
- [ ] Dashboard page structure
- [ ] Stats cards (total orders, active bots, P&L, balance)
- [ ] Recent orders section
- [ ] Quick actions section
- [ ] Market overview widget

**Time: 6-8 hours**

### 5.2 Dashboard Components
- [ ] StatsCard component (icon, title, value, change)
- [ ] RecentOrders table component
- [ ] QuickActions buttons (trade, view market, manage bots)
- [ ] MarketOverview widget (top pumps, top gaps)
- [ ] Real-time updates via WebSocket

**Time: 6-8 hours**

### 5.3 Data Integration
- [ ] Fetch dashboard summary data
- [ ] Real-time balance updates via WebSocket
- [ ] Real-time order updates via WebSocket
- [ ] Polling fallback if WebSocket disconnected

**Time: 4-6 hours**

---

## Phase 6: Market Analysis Feature
**Estimated Time: 3-4 days**

### 6.1 Market List Page
- [ ] Market page layout
- [ ] Market table with all pairs
- [ ] Search/filter by pair name
- [ ] Sort by pump score, gap %, change 24h, volume
- [ ] Pagination support
- [ ] Refresh button (manual refresh)

**Time: 6-8 hours**

### 6.2 Market Table Components
- [ ] MarketTable component with sorting
- [ ] PumpScoreBadge component (color-coded by score)
- [ ] Change24h component (green/red with arrow)
- [ ] VolumeDisplay component (formatted IDR)
- [ ] GapPercentage component
- [ ] Trade button (opens trade dialog)

**Time: 6-8 hours**

### 6.3 Market Detail Page
- [ ] Pair detail page (individual coin page)
- [ ] Price chart (optional, using Recharts or simple display)
- [ ] Timeframe cards (1m, 5m, 15m, 30m data)
- [ ] Pump score breakdown
- [ ] Gap analysis display
- [ ] Recent trades list
- [ ] Quick trade button

**Time: 8-10 hours**

### 6.4 Real-time Updates
- [ ] WebSocket integration for market data
- [ ] Subscribe to market_summary channel
- [ ] Update table in real-time (price, pump score, gap)
- [ ] Visual indicators for price changes (flash animation)
- [ ] Sound/notification on high pump score (optional)

**Time: 6-8 hours**

---

## Phase 7: Trading Feature (Copilot)
**Estimated Time: 3-4 days**

### 7.1 Trade Form
- [ ] TradeForm component (buy order)
- [ ] Pair selection (dropdown)
- [ ] Buying price input
- [ ] Volume (IDR) input
- [ ] Target profit (%) input
- [ ] Stop-loss (%) input
- [ ] Amount calculation display (coins to buy)
- [ ] Expected sell price display
- [ ] Balance validation before submit
- [ ] Form validation (Zod schema)

**Time: 6-8 hours**

### 7.2 Orders Management
- [ ] OrdersTable component (all orders)
- [ ] Filter by status (all, open, filled, cancelled)
- [ ] Filter by pair
- [ ] Order status badges (color-coded)
- [ ] Cancel button (for open orders)
- [ ] Sell Now button (for filled buy orders)
- [ ] Order detail modal/dialog
- [ ] Pagination support

**Time: 8-10 hours**

### 7.3 Order Detail
- [ ] OrderDetail component (full order info)
- [ ] Order lifecycle display (pending → open → filled)
- [ ] Linked orders display (buy ↔ sell)
- [ ] Profit/loss calculation display
- [ ] Target profit and stop-loss indicators
- [ ] Order actions (cancel, sell, view on exchange)

**Time: 4-6 hours**

### 7.4 Balance Display
- [ ] BalanceCard component
- [ ] Display IDR and coin balances
- [ ] Available vs frozen display
- [ ] Refresh balance button
- [ ] Real-time balance updates via WebSocket

**Time: 4-5 hours**

### 7.5 Trade Dialog/Modal
- [ ] Trade dialog (opened from market table)
- [ ] Pre-fill pair from market selection
- [ ] Pre-fill price with current market price
- [ ] Quick trade option (market price, default settings)
- [ ] Advanced trade option (custom settings)

**Time: 4-6 hours**

### 7.6 Real-time Order Updates
- [ ] WebSocket integration for order updates
- [ ] Subscribe to order_update channel
- [ ] Update orders table in real-time
- [ ] Toast notifications on order fill
- [ ] Alert notification on stop-loss trigger
- [ ] Sound notification (optional)

**Time: 6-8 hours**

---

## Phase 8: Bot Management Features
**Estimated Time: 4-5 days**

### 8.1 Bot List Page
- [ ] Bots page layout
- [ ] Tabs for bot types (Market Maker, Pump Hunter, Copilot)
- [ ] Bot list table (status, pair, P&L, created)
- [ ] Create bot button (per type)
- [ ] Bot status badges (running, stopped, error)
- [ ] Quick actions (start, stop, view details)

**Time: 6-8 hours**

### 8.2 Market Maker Bot
- [ ] Create Market Maker bot form
- [ ] Bot configuration fields (pair, spread target, order size, limits)
- [ ] Paper trading toggle
- [ ] Validation for bot parameters
- [ ] Bot detail page (config, stats, P&L, recent trades)
- [ ] Start/stop bot controls
- [ ] Edit bot configuration
- [ ] Delete bot with confirmation

**Time: 10-12 hours**

### 8.3 Pump Hunter Bot
- [ ] Create Pump Hunter bot form
- [ ] Bot configuration fields (pump score threshold, position limits, risk settings)
- [ ] Entry/exit condition configuration
- [ ] Paper trading toggle
- [ ] Bot detail page (config, active positions, closed positions)
- [ ] Position cards (entry, current P&L, exit targets)
- [ ] Manual position close button
- [ ] Start/stop bot controls
- [ ] Edit bot configuration
- [ ] Delete bot with confirmation

**Time: 10-12 hours**

### 8.4 Bot Statistics Dashboard
- [ ] Bot performance summary (total P&L, win rate, trades)
- [ ] P&L chart (daily/hourly)
- [ ] Trade history table
- [ ] Bot-specific metrics (spread captured, avg hold time, etc.)
- [ ] Export bot data (CSV)

**Time: 6-8 hours**

### 8.5 Real-time Bot Updates
- [ ] WebSocket integration for bot status
- [ ] Subscribe to bot_update channel
- [ ] Real-time P&L updates
- [ ] Real-time position updates (Pump Hunter)
- [ ] Toast notifications on bot events (started, stopped, error)

**Time: 4-6 hours**

---

## Phase 9: API Key Management
**Estimated Time: 1-2 days**

### 9.1 API Key Settings
- [ ] Settings page layout
- [ ] API Key section in settings
- [ ] Display API key status (configured, valid, invalid)
- [ ] Add/Edit API key form
- [ ] API key input fields (key + secret)
- [ ] Test API key button (validate with Indodax)
- [ ] Delete API key with confirmation
- [ ] Security warning messages

**Time: 6-8 hours**

### 9.2 API Key Validation
- [ ] Real-time validation on input
- [ ] Test connection with backend
- [ ] Display validation result (success, error message)
- [ ] Show account info on successful validation
- [ ] Loading state during validation

**Time: 3-4 hours**

---

## Phase 10: Admin Panel
**Estimated Time: 2-3 days**

### 10.1 User Management
- [ ] Admin panel layout (separate from main app)
- [ ] User list table (all users)
- [ ] User filters (role, status, has API key)
- [ ] User search
- [ ] Create user button
- [ ] Edit user button (opens dialog)
- [ ] Delete user button (with confirmation)
- [ ] Reset password button

**Time: 8-10 hours**

### 10.2 User Forms
- [ ] CreateUser form (username, email, password, role)
- [ ] EditUser form (email, role, status)
- [ ] Form validation (Zod)
- [ ] Success/error handling
- [ ] Toast notifications

**Time: 4-6 hours**

### 10.3 System Statistics (Optional)
- [ ] System stats dashboard (total users, active bots, orders)
- [ ] Redis stats (memory, keys)
- [ ] API usage stats
- [ ] Recent activity log

**Time: 4-6 hours**

---

## Phase 11: Settings & Profile
**Estimated Time: 1-2 days**

### 11.1 User Profile
- [ ] Profile page layout
- [ ] Display user info (username, email, role)
- [ ] Edit profile form (email only)
- [ ] Change password form (old password, new password, confirm)
- [ ] Form validation

**Time: 4-6 hours**

### 11.2 Theme & Preferences
- [ ] Theme toggle (light/dark mode)
- [ ] Notification preferences (sound, toast, email)
- [ ] Language selection (if i18n added later)
- [ ] Default trading settings (for quick trade)

**Time: 3-4 hours**

---

## Phase 12: Utilities & Helpers
**Estimated Time: 1-2 days**

### 12.1 Formatting Utilities
- [ ] Currency formatting (IDR, BTC, etc.)
- [ ] Number formatting (compact: 1.5M, 2.3K)
- [ ] Date/time formatting (relative: "2 hours ago")
- [ ] Percentage formatting
- [ ] Decimal precision utilities

**Time: 3-4 hours**

### 12.2 Validation Utilities
- [ ] Email validation
- [ ] Password strength checker
- [ ] Number range validation
- [ ] Custom Zod validators

**Time: 2-3 hours**

### 12.3 Custom Hooks
- [ ] useDebounce hook (search inputs)
- [ ] usePagination hook (table pagination)
- [ ] useLocalStorage hook (persist state)
- [ ] useMediaQuery hook (responsive design)
- [ ] useClickOutside hook (close dropdowns)
- [ ] useInterval hook (polling)

**Time: 4-6 hours**

---

## Phase 13: Error Handling & Loading States
**Estimated Time: 1-2 days**

### 13.1 Error Boundary
- [ ] ErrorBoundary component (catch React errors)
- [ ] Error fallback UI
- [ ] Error reporting (console, optional Sentry)
- [ ] Reset error button

**Time: 3-4 hours**

### 13.2 Loading States
- [ ] Global loading indicator (page transitions)
- [ ] Skeleton loaders for tables
- [ ] Skeleton loaders for cards
- [ ] Spinner component variants
- [ ] Suspense fallbacks

**Time: 4-5 hours**

### 13.3 Empty States
- [ ] Empty state component (generic)
- [ ] No orders empty state
- [ ] No bots empty state
- [ ] No search results empty state
- [ ] Call-to-action buttons in empty states

**Time: 3-4 hours**

---

## Phase 14: Responsive Design & Mobile
**Estimated Time: 2-3 days**

### 14.1 Mobile Navigation
- [ ] Hamburger menu component
- [ ] Mobile sidebar (slide from left)
- [ ] Mobile header (compact)
- [ ] Bottom navigation (optional)

**Time: 4-6 hours**

### 14.2 Responsive Tables
- [ ] Mobile card view for tables
- [ ] Horizontal scroll for wide tables
- [ ] Collapsible table columns on mobile
- [ ] Touch-friendly action buttons

**Time: 6-8 hours**

### 14.3 Responsive Forms
- [ ] Stack form inputs on mobile
- [ ] Full-width buttons on mobile
- [ ] Touch-friendly input sizes
- [ ] Mobile-friendly dialogs (full-screen)

**Time: 4-6 hours**

---

## Phase 15: Performance Optimization
**Estimated Time: 1-2 days**

### 15.1 Code Splitting
- [ ] Route-based code splitting (lazy loading)
- [ ] Component lazy loading (heavy components)
- [ ] Dynamic imports for large libraries
- [ ] Manual chunk splitting (vendor, ui, query)

**Time: 4-5 hours**

### 15.2 React Query Optimization
- [ ] Configure stale time and cache time
- [ ] Implement optimistic updates
- [ ] Query prefetching (on hover)
- [ ] Query deduplication
- [ ] Pagination with infinite scroll (if needed)

**Time: 4-6 hours**

### 15.3 Performance Monitoring
- [ ] Measure component render times
- [ ] Identify slow components (React DevTools Profiler)
- [ ] Optimize heavy re-renders (useMemo, useCallback)
- [ ] Lazy load images (if used)

**Time: 3-4 hours**

---

## Phase 16: Testing
**Estimated Time: 3-4 days**

### 16.1 Component Tests
- [ ] Test UI components (Button, Input, etc.)
- [ ] Test form validation (TradeForm, LoginForm)
- [ ] Test table interactions (sorting, filtering)
- [ ] Test modal open/close
- [ ] Test theme toggle

**Time: 8-10 hours**

### 16.2 Integration Tests
- [ ] Test login flow (form submit, success, error)
- [ ] Test trade flow (place order, cancel, sell)
- [ ] Test bot creation and management
- [ ] Test API key validation flow
- [ ] Test WebSocket connection and updates

**Time: 10-12 hours**

### 16.3 E2E Tests (Optional)
- [ ] Setup Playwright or Cypress
- [ ] Test complete user journey (login → trade → logout)
- [ ] Test admin panel flows
- [ ] Test responsive design on different devices

**Time: 8-10 hours**

---

## Phase 17: Vercel Deployment
**Estimated Time: 1 day**

### 17.1 Deployment Configuration
- [ ] Create vercel.json configuration
- [ ] Configure environment variables (VITE_API_URL, VITE_WS_URL)
- [ ] Setup rewrites for SPA routing
- [ ] Configure cache headers for assets
- [ ] Setup build command and output directory

**Time: 2-3 hours**

### 17.2 Vercel Deployment
- [ ] Connect GitHub repository to Vercel
- [ ] Configure production environment variables
- [ ] Configure preview environment variables
- [ ] Test preview deployment
- [ ] Deploy to production
- [ ] Verify production build

**Time: 2-3 hours**

### 17.3 CI/CD (Optional)
- [ ] Setup GitHub Actions for testing
- [ ] Auto-deploy on push to main (production)
- [ ] Auto-deploy on push to develop (preview)
- [ ] Run tests before deployment
- [ ] Notify on deployment success/failure

**Time: 3-4 hours**

---

## Phase 18: Documentation & Polish
**Estimated Time: 1-2 days**

### 18.1 Code Documentation
- [ ] Add JSDoc comments to complex functions
- [ ] Document component props (TypeScript interfaces)
- [ ] Add README.md for frontend folder
- [ ] Document environment variables
- [ ] Document build and deployment process

**Time: 4-5 hours**

### 18.2 UI/UX Polish
- [ ] Consistent spacing and sizing
- [ ] Smooth transitions and animations
- [ ] Loading state transitions
- [ ] Error message consistency
- [ ] Success message consistency
- [ ] Accessibility improvements (ARIA labels, keyboard navigation)

**Time: 6-8 hours**

---

## Summary

### Total Estimated Time: **30-40 days** (solo developer, full-time)

### Phase Breakdown:
1. **Project Setup & Configuration**: 1-2 days
2. **Core Infrastructure**: 2-3 days
3. **UI Component Library**: 3-4 days
4. **Authentication Feature**: 2-3 days
5. **Dashboard Page**: 2-3 days
6. **Market Analysis Feature**: 3-4 days
7. **Trading Feature (Copilot)**: 3-4 days
8. **Bot Management Features**: 4-5 days
9. **API Key Management**: 1-2 days
10. **Admin Panel**: 2-3 days
11. **Settings & Profile**: 1-2 days
12. **Utilities & Helpers**: 1-2 days
13. **Error Handling & Loading States**: 1-2 days
14. **Responsive Design & Mobile**: 2-3 days
15. **Performance Optimization**: 1-2 days
16. **Testing**: 3-4 days
17. **Vercel Deployment**: 1 day
18. **Documentation & Polish**: 1-2 days

### Critical Path:
1. Setup → Infrastructure → UI Components → Auth → Dashboard
2. Market Analysis and Trading can be developed in parallel (after infrastructure)
3. Bot Management depends on Trading feature
4. Testing can be incremental throughout development

### Parallelization Opportunities:
- UI components can be built in parallel with infrastructure
- Market Analysis and Trading features can be developed simultaneously
- Testing can be done alongside feature development
- Documentation can be written during development

### Risk Factors:
- **Radix UI learning curve**: May need extra time to customize (1-2 days buffer)
- **WebSocket integration complexity**: May need debugging (1-2 days buffer)
- **Real-time updates optimization**: May need performance tuning (1-2 days buffer)
- **Responsive design edge cases**: May need extra polish (1-2 days buffer)

### Recommended Approach:
1. **MVP First** (Phases 1-7): ~15-20 days
   - Core functionality with auth, market, and basic trading
   - Get working UI deployed
   - Gather user feedback
   
2. **Advanced Features** (Phases 8-11): ~8-12 days
   - Bot management, API keys, admin panel
   - More complex UI interactions
   
3. **Polish & Production** (Phases 12-18): ~7-10 days
   - Testing, optimization, deployment
   - Mobile responsiveness
   - Production-ready polish

---

## Priority Levels

### 🔴 Critical (Must Have for MVP)
- Authentication (login, register, protected routes)
- Market Analysis page (list, sort, real-time)
- Trading feature (trade form, orders list, basic management)
- Basic dashboard
- API service layer
- WebSocket integration

### 🟡 Important (High Value)
- Bot management (Market Maker, Pump Hunter)
- Real-time order updates
- Mobile responsive design
- Admin panel
- API key management

### 🟢 Nice to Have (Future Enhancements)
- Advanced charts (TradingView integration)
- PWA support
- Push notifications
- Export data features
- Advanced analytics

---

**Note**: Time estimates assume:
- Experienced React/TypeScript developer
- Familiarity with React Query and form libraries
- Some crypto trading UI knowledge
- Working independently
- No major blockers or design changes

**Adjustment factors**:
- Junior developer: Add 50-100% more time
- Team of 2-3: Reduce total time by 40-60% (parallelization)
- Part-time: Multiply by schedule factor
- With pre-made design system: Reduce UI component time by 30-40%

