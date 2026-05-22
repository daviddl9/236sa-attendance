import * as React from 'react';
import { cn } from '@/lib/utils';

// NoPasteInput renders an <input> that blocks paste, drop, and
// right-click context menu on the field. Used for the NRIC Last 5
// confirmation field so the user must type the value manually.
// All other props (className, type, value, onChange, …) are forwarded.
const NoPasteInput = React.forwardRef<HTMLInputElement, React.ComponentProps<'input'>>(
  ({ className, onPaste, onDrop, onContextMenu, ...props }, ref) => {
    const block = (e: React.SyntheticEvent) => e.preventDefault();

    return (
      <input
        ref={ref}
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
        spellCheck={false}
        onPaste={(e) => { block(e); onPaste?.(e); }}
        onDrop={(e) => { block(e); onDrop?.(e); }}
        onContextMenu={(e) => { block(e); onContextMenu?.(e); }}
        className={cn(
          'file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input flex h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
          'focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]',
          'aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive',
          className,
        )}
        {...props}
      />
    );
  },
);
NoPasteInput.displayName = 'NoPasteInput';

export { NoPasteInput };
