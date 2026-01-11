import { ReactNode } from 'react';

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-12 px-4">
      {icon && (
        <div className="flex items-center justify-center mb-4">
          {icon}
        </div>
      )}
      <h3 className="text-lg font-semibold text-white mb-2">{title}</h3>
      {description && (
        <p className="text-sm text-gray-400 text-center max-w-md mb-4">{description}</p>
      )}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

// Preset empty states
export function NoDataEmptyState() {
  return (
    <EmptyState
      icon={
        <img src="/tuyul-crying.png" alt="Crying Tuyul" className="w-64 h-auto" />
      }
      title="I Can't Find Anything!"
      description="There's no data here, Master. I looked everywhere but my magic bag is empty! Check back later..."
    />
  );
}

export function NoBotsEmptyState({ onCreate }: { onCreate?: () => void }) {
  return (
    <EmptyState
      icon={
        <img src="/tuyul-crying.png" alt="Crying Tuyul" className="w-64 h-auto" />
      }
      title="Master, I Have No Helpers!"
      description="I can't steal money without my little helpers... Give me some helpers so I can work for you 24/7 and steal all that money! 💰"
      action={
        onCreate && (
          <button
            onClick={onCreate}
            className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors"
          >
            Give Me My First Helper! 🤖
          </button>
        )
      }
    />
  );
}

export function NoTradesEmptyState() {
  return (
    <EmptyState
      icon={
        <img src="/tuyul-crying.png" alt="Crying Tuyul" className="w-64 h-auto" />
      }
      title="I Haven't Stolen Anything Yet!"
      description="No trades yet, Master. Once I start my stealing missions... I mean, trading for you, all the loot will show up here! 💰"
    />
  );
}

export function ConnectionErrorEmptyState({ onRetry }: { onRetry?: () => void }) {
  return (
    <EmptyState
      icon={
        <img src="/tuyul-crying.png" alt="Crying Tuyul" className="w-64 h-auto" />
      }
      title="Master! I Can't Connect!"
      description="My magic isn't working... I can't reach the treasure vault! Please check your internet connection and let me try again!"
      action={
        onRetry && (
          <button
            onClick={onRetry}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-white rounded-lg font-medium transition-colors"
          >
            Let Me Try Again! 🔄
          </button>
        )
      }
    />
  );
}

