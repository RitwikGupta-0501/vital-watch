import type { FC, HTMLAttributes } from 'react';
import { clsx } from 'clsx';
import type { StatusMeta, StatusTone } from '../../lib/status';

interface StatusBadgeProps extends HTMLAttributes<HTMLSpanElement> {
  meta: StatusMeta;
  size?: 'sm' | 'md' | 'lg';
  isPatientTheme?: boolean;
  showIcon?: boolean;
}

const TONE_CLASSES: Record<StatusTone, string> = {
  neutral: 'bg-slate-100 text-slate-800 border-slate-200',
  info: 'bg-sky-50 text-sky-800 border-sky-200',
  success: 'bg-emerald-50 text-emerald-800 border-emerald-200',
  warning: 'bg-amber-50 text-amber-900 border-amber-200',
  critical: 'bg-rose-50 text-rose-800 border-rose-200 font-semibold',
};

const ICON_SIZES: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'w-3 h-3',
  md: 'w-3.5 h-3.5',
  lg: 'w-4 h-4',
};

const TEXT_SIZES: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'text-xs px-2 py-0.5 gap-1',
  md: 'text-xs px-2.5 py-1 gap-1.5',
  lg: 'text-sm px-3 py-1.5 gap-2',
};

export const StatusBadge: FC<StatusBadgeProps> = ({
  meta,
  size = 'md',
  isPatientTheme = false,
  showIcon = true,
  className,
  ...props
}) => {
  const Icon = meta.icon;
  const toneClass = TONE_CLASSES[meta.tone] || TONE_CLASSES.neutral;

  return (
    <span
      className={clsx(
        'inline-flex items-center font-medium border select-none transition-colors',
        isPatientTheme ? 'rounded-full' : 'rounded-md',
        toneClass,
        TEXT_SIZES[size],
        className
      )}
      title={meta.description || meta.label}
      {...props}
    >
      {showIcon && Icon && <Icon className={clsx(ICON_SIZES[size], 'shrink-0')} aria-hidden="true" />}
      <span>{meta.label}</span>
    </span>
  );
};
