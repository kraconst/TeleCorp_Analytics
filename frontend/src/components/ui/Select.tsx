import { ChevronDown } from 'lucide-react';
import { forwardRef, type SelectHTMLAttributes } from 'react';
import { cn } from '@/lib/utils';

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, children, ...props }, ref) => (
    <span className="relative inline-flex min-w-0 items-center">
      <select
        ref={ref}
        className={cn(
          'h-9 w-full min-w-0 appearance-none rounded-lg border border-stone-300 bg-white py-0 pl-3 pr-8 text-sm text-stone-700 disabled:bg-stone-100 disabled:text-stone-500',
          className,
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-2.5 size-4 text-stone-500" aria-hidden="true" />
    </span>
  ),
);
Select.displayName = 'Select';
