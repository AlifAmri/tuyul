import { Navigate } from 'react-router-dom';

// Redirect /market to /market (AllMarketsPage) for backward compatibility
export function MarketPage() {
  return <Navigate to="/market" replace />;
}

