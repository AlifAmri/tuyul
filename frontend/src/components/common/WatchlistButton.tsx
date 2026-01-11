import { useWatchlistStore } from '@/stores/watchlistStore';
import { cn } from '@/utils/cn';

interface WatchlistButtonProps {
  pair: string;
  className?: string;
}

export function WatchlistButton({ pair, className }: WatchlistButtonProps) {
  const { isPairWatched, togglePair } = useWatchlistStore();
  const isWatched = isPairWatched(pair);

  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        togglePair(pair);
      }}
      className={cn(
        'p-2 rounded-lg transition-colors',
        isWatched
          ? 'text-yellow-500 hover:text-yellow-400 bg-yellow-500/10'
          : 'text-gray-400 hover:text-yellow-500 hover:bg-gray-800',
        className
      )}
      aria-label={isWatched ? 'Remove from watchlist' : 'Add to watchlist'}
      title={isWatched ? 'Remove from watchlist' : 'Add to watchlist'}
    >
      <svg
        className="w-5 h-5"
        fill={isWatched ? 'currentColor' : 'none'}
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"
        />
      </svg>
    </button>
  );
}

