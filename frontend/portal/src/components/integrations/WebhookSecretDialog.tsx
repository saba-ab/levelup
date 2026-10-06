import { AlertTriangle } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import type { WebhookEndpointWithSecret } from '@/services/api/models/webhooks';
import { CopyButton } from './CodeBlock';

interface WebhookSecretDialogProps {
  /** The create or rotate-secret response; null closes the dialog. */
  endpoint: WebhookEndpointWithSecret | null;
  reason: 'created' | 'rotated';
  onClose: () => void;
}

/** Shows a signing secret the one time the API returns it. */
export function WebhookSecretDialog({ endpoint, reason, onClose }: WebhookSecretDialogProps) {
  return (
    <Dialog open={!!endpoint} onOpenChange={open => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{reason === 'created' ? 'Endpoint added' : 'Signing secret rotated'}</DialogTitle>
          <DialogDescription className="break-all">{endpoint?.url}</DialogDescription>
        </DialogHeader>
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            This is the only time the signing secret is shown. Store it in your secret manager now; if you lose it,
            rotate it to get a new one.
            {reason === 'rotated' && ' Deliveries from now on are signed with this secret.'}
          </AlertDescription>
        </Alert>
        <div className="flex items-center gap-2">
          <Input
            readOnly
            value={endpoint?.secret ?? ''}
            className="font-mono text-xs"
            aria-label="Signing secret"
            onFocus={e => e.target.select()}
          />
          <CopyButton text={endpoint?.secret ?? ''} label="Copy signing secret" className="h-9 w-9" />
        </div>
        <DialogFooter>
          <Button onClick={onClose}>I have stored it</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
