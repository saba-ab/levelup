import { useNavigate } from 'react-router-dom';
import { AlertTriangle, Bell, PlugZap } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuth } from '@/contexts/AuthContext';
import { useWebhookDeliveriesQuery, useWebhookEndpointsQuery } from '@/services/queries/webhooks';

/**
 * Header bell: operator alerts the tenant should act on — disabled webhook
 * endpoints and recently failed deliveries. Player-facing notifications live
 * on the Notifications page.
 */
export default function OperatorAlerts() {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();
  const canView = hasPermission('view:integrations');
  const endpoints = useWebhookEndpointsQuery({ limit: 100 }, canView);
  const failed = useWebhookDeliveriesQuery({ status: 'failed', limit: 5 }, canView);

  const disabled = (endpoints.data?.data ?? []).filter((e) => !e.is_active && e.disabled_reason);
  const failures = failed.data?.data ?? [];
  const count = disabled.length + failures.length;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={count ? `${count} alerts` : 'Alerts'} className="relative">
          <Bell className="w-5 h-5" />
          {count > 0 && (
            <span className="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-destructive" />
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <DropdownMenuLabel>Alerts</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {!canView && (
          <p className="px-3 py-4 text-sm text-muted-foreground">No alerts for your role.</p>
        )}
        {canView && count === 0 && (
          <p className="px-3 py-4 text-sm text-muted-foreground">
            {endpoints.isLoading || failed.isLoading ? 'Checking…' : 'All clear: webhooks are delivering.'}
          </p>
        )}
        {disabled.map((e) => (
          <DropdownMenuItem key={e.id} onClick={() => navigate('/integrations')} className="items-start gap-2">
            <PlugZap className="w-4 h-4 mt-0.5 text-destructive shrink-0" />
            <div className="min-w-0">
              <p className="text-sm font-medium">Webhook endpoint disabled</p>
              <p className="text-xs text-muted-foreground truncate">{e.url}</p>
            </div>
          </DropdownMenuItem>
        ))}
        {failures.map((d) => (
          <DropdownMenuItem key={d.id} onClick={() => navigate('/audit-logs')} className="items-start gap-2">
            <AlertTriangle className="w-4 h-4 mt-0.5 text-amber-500 shrink-0" />
            <div className="min-w-0">
              <p className="text-sm font-medium">Delivery failed: {d.event}</p>
              <p className="text-xs text-muted-foreground truncate">
                {d.response_status ? `HTTP ${d.response_status}` : d.last_error || 'no response'} · {d.attempts} attempts
              </p>
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
