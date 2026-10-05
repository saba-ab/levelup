import { ReactNode } from 'react';
import { Construction } from 'lucide-react';
import { cn } from '@/lib/utils';

interface FeatureUnavailableProps {
  /** e.g. "Segments" */
  feature: string;
  /** Extra sentence on what to use instead, if anything. */
  hint?: ReactNode;
  /**
   * The page's layout, kept as a preview. It is dimmed and non-interactive:
   * its figures are illustrative, never data from the API.
   */
  children?: ReactNode;
  className?: string;
}

/**
 * "Not available in this API version" state for portal pages whose backend
 * module does not exist in the Go API yet. Nothing on the page calls an
 * endpoint; the preview below the banner cannot be interacted with.
 */
export default function FeatureUnavailable({ feature, hint, children, className }: FeatureUnavailableProps) {
  return (
    <div className={cn('space-y-6', className)}>
      <div
        role="status"
        className="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-4 text-amber-600 dark:text-amber-400"
      >
        <Construction className="mt-0.5 h-5 w-5 shrink-0" />
        <div className="space-y-1">
          <p className="font-medium">{feature} is coming soon</p>
          <p className="text-sm opacity-90">
            {feature} is not available in this API version. The preview below shows the planned layout with
            illustrative sample values only; it is not your data and its actions are disabled.
          </p>
          {hint && <p className="text-sm opacity-90">{hint}</p>}
        </div>
      </div>
      {children && (
        <div
          aria-hidden="true"
          ref={el => el?.setAttribute('inert', '')}
          className="pointer-events-none select-none opacity-50 grayscale-[40%]">
          {children}
        </div>
      )}
    </div>
  );
}
