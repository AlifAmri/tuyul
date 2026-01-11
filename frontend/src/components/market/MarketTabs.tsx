import { Link, useLocation } from 'react-router-dom';
import { cn } from '@/utils/cn';

export function MarketTabs() {
  const location = useLocation();
  const currentPath = location.pathname;

  const tabs = [
    { name: 'All Markets', path: '/market' },
    { name: 'Pump Scores', path: '/market/pumps' },
    { name: 'Gaps', path: '/market/gaps' },
  ];

  return (
    <div className="flex gap-2">
      {tabs.map((tab) => (
        <Link
          key={tab.path}
          to={tab.path}
          className={cn(
            'px-4 py-2 rounded-lg font-medium transition-all',
            currentPath === tab.path
              ? 'bg-primary-600 text-white'
              : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
          )}
        >
          {tab.name}
        </Link>
      ))}
    </div>
  );
}

