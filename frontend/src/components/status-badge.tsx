import { Badge } from './ui/badge';
import { cn } from '@/lib/utils';
import { STATUS_DISPLAY_CONFIG, type StatusType } from '@/lib/api-client';

interface StatusBadgeProps {
  statusType: StatusType;
  className?: string;
}

export function StatusBadge({ statusType, className }: StatusBadgeProps) {
  const config = STATUS_DISPLAY_CONFIG[statusType];

  return (
    <Badge className={cn('text-white border-0', config.color, className)}>
      {config.label}
    </Badge>
  );
}
