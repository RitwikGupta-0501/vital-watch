import React from 'react';
import { clsx } from 'clsx';
import { FileQuestion, CalendarX2, ShieldAlert, AlertTriangle } from 'lucide-react';
import { Button } from './Button';

export type EmptyStateVariant = 'no-records' | 'not-configured' | 'rate-limited' | 'error';

interface EmptyStateProps {
  variant?: EmptyStateVariant;
  title?: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  isPatientTheme?: boolean;
  className?: string;
}

const DEFAULT_CONFIG: Record<
  EmptyStateVariant,
  { title: string; description: string; icon: React.ComponentType<{ className?: string }> }
> = {
  'no-records': {
    title: 'No records found',
    description: 'There is currently no data recorded for this view.',
    icon: FileQuestion,
  },
  'not-configured': {
    title: 'Schedule not available',
    description: 'The clinician has not yet configured available booking hours.',
    icon: CalendarX2,
  },
  'rate-limited': {
    title: 'Too many requests',
    description: 'Security rate limit engaged. Please wait before retrying.',
    icon: ShieldAlert,
  },
  error: {
    title: 'Unable to load content',
    description: 'An unexpected error occurred while communicating with clinical services.',
    icon: AlertTriangle,
  },
};

export const EmptyState: React.FC<EmptyStateProps> = ({
  variant = 'no-records',
  title,
  description,
  actionLabel,
  onAction,
  isPatientTheme = false,
  className,
}) => {
  const config = DEFAULT_CONFIG[variant];
  const Icon = config.icon;

  return (
    <div
      className={clsx(
        'flex flex-col items-center justify-center p-8 md:p-12 text-center border border-dashed border-border bg-surface/50',
        isPatientTheme ? 'rounded-3xl' : 'rounded-lg',
        className
      )}
    >
      <div className="w-12 h-12 rounded-full bg-slate-100 flex items-center justify-center text-slate-500 mb-4">
        <Icon className="w-6 h-6" />
      </div>

      <h3 className="text-base md:text-lg font-semibold text-slate-900 tracking-tight mb-1">
        {title || config.title}
      </h3>

      <p className="text-sm text-slate-500 max-w-sm mb-6 leading-relaxed">
        {description || config.description}
      </p>

      {actionLabel && onAction && (
        <Button
          onClick={onAction}
          variant="outline"
          size="sm"
          isPatientTheme={isPatientTheme}
        >
          {actionLabel}
        </Button>
      )}
    </div>
  );
};
