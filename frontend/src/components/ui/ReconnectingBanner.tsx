import type { FC } from 'react';
import { RefreshCw, WifiOff } from 'lucide-react';
import type { SSEConnectionStatus } from '../../hooks/useNotificationStream';

interface ReconnectingBannerProps {
  status: SSEConnectionStatus;
  onRetry?: () => void;
}

export const ReconnectingBanner: FC<ReconnectingBannerProps> = ({
  status,
  onRetry,
}) => {
  if (status === 'connected' || status === 'connecting') {
    return null;
  }

  return (
    <div
      role="status"
      aria-live="polite"
      className="bg-amber-500 text-slate-950 px-4 py-2 text-xs md:text-sm font-medium flex items-center justify-between sticky top-0 z-50 shadow-sm border-b border-amber-600/20"
    >
      <div className="flex items-center gap-2">
        <WifiOff className="w-4 h-4 shrink-0 animate-pulse text-amber-950" />
        <span>
          {status === 'reconnecting'
            ? 'Live updates disconnected. Reconnecting to clinical stream...'
            : 'Live stream disconnected.'}
        </span>
      </div>
      {onRetry && (
        <button
          onClick={onRetry}
          type="button"
          className="inline-flex items-center gap-1.5 px-2.5 py-0.5 bg-amber-600/30 hover:bg-amber-600/50 text-slate-950 rounded text-xs font-semibold cursor-pointer transition-colors"
        >
          <RefreshCw className="w-3 h-3" />
          <span>Retry Now</span>
        </button>
      )}
    </div>
  );
};
