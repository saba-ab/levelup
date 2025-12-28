import React from 'react';
import { useConnectionStatus } from '@/hooks/useConnectionStatus';
import { cn } from '@/lib/utils';
import { Wifi, WifiOff, RefreshCw, Clock } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';

interface ConnectionStatusIndicatorProps {
  showLabel?: boolean;
  showLatency?: boolean;
  size?: 'sm' | 'md';
  className?: string;
}

export default function ConnectionStatusIndicator({
  showLabel = false,
  showLatency = true,
  size = 'sm',
  className,
}: ConnectionStatusIndicatorProps) {
  const { isOnline, latency, lastChecked, error, isChecking, refresh, environment } = useConnectionStatus();

  const getLatencyColor = (ms: number) => {
    if (ms < 100) return 'text-green-400';
    if (ms < 300) return 'text-amber-400';
    return 'text-red-400';
  };

  const formatLastChecked = (date: Date | null) => {
    if (!date) return 'Never';
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    return `${minutes}m ago`;
  };

  const statusDotSize = size === 'sm' ? 'w-2 h-2' : 'w-2.5 h-2.5';
  const iconSize = size === 'sm' ? 'w-3.5 h-3.5' : 'w-4 h-4';

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className={cn(
            "flex items-center gap-1.5 px-2 py-1 rounded-md bg-background/50 border border-border/50 cursor-default",
            className
          )}
        >
          {/* Status dot */}
          <div className="relative flex items-center justify-center">
            <span
              className={cn(
                statusDotSize,
                "rounded-full",
                isOnline ? "bg-green-500" : "bg-red-500",
                isChecking && "animate-pulse"
              )}
            />
            {isOnline && (
              <span
                className={cn(
                  statusDotSize,
                  "absolute rounded-full bg-green-500 animate-ping opacity-75"
                )}
                style={{ animationDuration: '2s' }}
              />
            )}
          </div>

          {/* Label */}
          {showLabel && (
            <span className={cn(
              "text-xs font-medium",
              isOnline ? "text-green-400" : "text-red-400"
            )}>
              {isOnline ? 'Online' : 'Offline'}
            </span>
          )}

          {/* Latency */}
          {showLatency && isOnline && latency !== null && (
            <span className={cn("text-[10px] font-mono", getLatencyColor(latency))}>
              {latency}ms
            </span>
          )}
        </div>
      </TooltipTrigger>
      <TooltipContent side="top" className="w-64 p-0" sideOffset={8}>
        <div className="p-3 space-y-3">
          {/* Header */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              {isOnline ? (
                <Wifi className={cn(iconSize, "text-green-400")} />
              ) : (
                <WifiOff className={cn(iconSize, "text-red-400")} />
              )}
              <span className="font-medium text-sm">
                {isOnline ? 'Connected' : 'Disconnected'}
              </span>
            </div>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              onClick={(e) => {
                e.stopPropagation();
                refresh();
              }}
              disabled={isChecking}
            >
              <RefreshCw className={cn("h-3 w-3", isChecking && "animate-spin")} />
            </Button>
          </div>

          {/* Details */}
          <div className="space-y-2 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">Environment</span>
              <span className="font-medium">{environment.name}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">Endpoint</span>
              <span className="font-mono text-[10px] truncate max-w-[140px]">{environment.url}</span>
            </div>
            {isOnline && latency !== null && (
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Latency</span>
                <span className={cn("font-mono", getLatencyColor(latency))}>{latency}ms</span>
              </div>
            )}
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground flex items-center gap-1">
                <Clock className="w-3 h-3" />
                Last checked
              </span>
              <span>{formatLastChecked(lastChecked)}</span>
            </div>
          </div>

          {/* Error message */}
          {error && (
            <div className="p-2 bg-destructive/10 rounded text-[10px] text-destructive">
              {error}
            </div>
          )}

          {/* Status message */}
          <div className={cn(
            "text-[10px] text-center py-1.5 rounded",
            isOnline ? "bg-green-500/10 text-green-400" : "bg-red-500/10 text-red-400"
          )}>
            {isOnline 
              ? 'Backend is responding normally' 
              : 'Cannot reach the backend server'}
          </div>
        </div>
      </TooltipContent>
    </Tooltip>
  );
}