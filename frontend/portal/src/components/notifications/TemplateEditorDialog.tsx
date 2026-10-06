import { useEffect, useRef, useState } from 'react';
import { Eye, Loader2, RefreshCw } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { PlayerPicker } from '@/components/players/PlayerPicker';
import { useToast } from '@/hooks/use-toast';
import { ApiRequestError } from '@/services/queries/rules';
import {
  useCreateNotificationTemplateMutation,
  useUpdateNotificationTemplateMutation,
  usePreviewNotificationTemplateMutation,
} from '@/services/queries/notifications';
import {
  NOTIFICATION_CHANNELS,
  NOTIFICATION_TRIGGERS,
  type NotificationChannel,
  type NotificationTemplate,
  type NotificationTrigger,
  type UpdateNotificationTemplateData,
} from '@/services/api/models/notifications';
import type { Player } from '@/services/api/types';
import { CHANNEL_LABELS, TRIGGER_LABELS, TRIGGER_VARIABLES, renderTemplateLocally } from './templateVariables';

interface TemplateEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Null to create a new template. */
  template: NotificationTemplate | null;
}

interface FormState {
  name: string;
  trigger: NotificationTrigger;
  channels: NotificationChannel[];
  title_template: string;
  body_template: string;
  is_active: boolean;
}

type FieldErrors = Partial<Record<keyof FormState | 'form', string>>;

const EMPTY: FormState = {
  name: '',
  trigger: 'badges.awarded',
  channels: ['in_app'],
  title_template: 'You earned the {{.Badge.Name}} badge!',
  body_template: 'Congratulations {{.Player.DisplayName}}, you just earned a {{.Badge.Tier}} badge.',
  is_active: true,
};

function toForm(t: NotificationTemplate | null): FormState {
  if (!t) return EMPTY;
  return {
    name: t.name,
    trigger: t.trigger,
    channels: t.channels,
    title_template: t.title_template,
    body_template: t.body_template,
    is_active: t.is_active,
  };
}

/** Only the fields that changed, for PATCH. */
function patchOf(original: NotificationTemplate, form: FormState): UpdateNotificationTemplateData {
  const patch: UpdateNotificationTemplateData = {};
  if (form.name.trim() !== original.name) patch.name = form.name.trim();
  if (form.trigger !== original.trigger) patch.trigger = form.trigger;
  if ([...form.channels].sort().join() !== [...original.channels].sort().join()) patch.channels = form.channels;
  if (form.title_template !== original.title_template) patch.title_template = form.title_template;
  if (form.body_template !== original.body_template) patch.body_template = form.body_template;
  if (form.is_active !== original.is_active) patch.is_active = form.is_active;
  return patch;
}

function fieldErrorsOf(err: unknown): FieldErrors {
  if (!(err instanceof ApiRequestError)) {
    return { form: err instanceof Error ? err.message : 'Failed to save template' };
  }
  if (err.code === 'notification_template_name_taken') {
    return { name: 'A template with this name already exists.' };
  }
  const out: FieldErrors = {};
  for (const [field, msgs] of Object.entries(err.validationErrors ?? {})) {
    const key = (field in EMPTY ? field : 'form') as keyof FieldErrors;
    out[key] = [out[key], msgs.join(', ')].filter(Boolean).join('; ');
  }
  if (Object.keys(out).length === 0) out.form = err.message;
  return out;
}

/** Create / edit a notification template with a variable picker and previews. */
export function TemplateEditorDialog({ open, onOpenChange, template }: TemplateEditorDialogProps) {
  const { toast } = useToast();
  const createMutation = useCreateNotificationTemplateMutation();
  const updateMutation = useUpdateNotificationTemplateMutation();
  const previewMutation = usePreviewNotificationTemplateMutation();
  const [form, setForm] = useState<FormState>(() => toForm(template));
  const [errors, setErrors] = useState<FieldErrors>({});
  const [previewPlayer, setPreviewPlayer] = useState<Player | null>(null);
  const [focused, setFocused] = useState<'title_template' | 'body_template'>('title_template');
  const titleRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const resetPreview = previewMutation.reset;

  useEffect(() => {
    if (open) {
      setForm(toForm(template));
      setErrors({});
      setPreviewPlayer(null);
      resetPreview();
    }
  }, [open, template, resetPreview]);

  const isSaving = createMutation.isPending || updateMutation.isPending;
  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
    setErrors((e) => ({ ...e, [key]: undefined }));
  };

  const toggleChannel = (ch: NotificationChannel, checked: boolean) =>
    set('channels', checked ? [...form.channels, ch] : form.channels.filter((c) => c !== ch));

  /** Inserts {{path}} at the cursor of the last focused source field. */
  const insertVariable = (path: string) => {
    const token = `{{${path}}}`;
    const el = focused === 'title_template' ? titleRef.current : bodyRef.current;
    const current = form[focused];
    const start = el?.selectionStart ?? current.length;
    const end = el?.selectionEnd ?? current.length;
    set(focused, current.slice(0, start) + token + current.slice(end));
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(start + token.length, start + token.length);
    });
  };

  const runServerPreview = () => {
    if (!template) return;
    previewMutation.mutate({ templateId: template.id, data: previewPlayer ? { player_id: previewPlayer.id } : undefined });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const local: FieldErrors = {};
    if (!form.name.trim()) local.name = 'Name is required.';
    if (form.channels.length === 0) local.channels = 'Pick at least one channel.';
    if (!form.title_template.trim()) local.title_template = 'Title is required.';
    if (Object.keys(local).length > 0) {
      setErrors(local);
      return;
    }
    try {
      if (template) {
        const patch = patchOf(template, form);
        if (Object.keys(patch).length === 0) {
          onOpenChange(false);
          return;
        }
        await updateMutation.mutateAsync({ templateId: template.id, data: patch });
        toast({ title: 'Template updated', description: form.name.trim() });
      } else {
        await createMutation.mutateAsync({ ...form, name: form.name.trim() });
        toast({ title: 'Template created', description: form.name.trim() });
      }
      onOpenChange(false);
    } catch (err) {
      setErrors(fieldErrorsOf(err));
      if (err instanceof ApiRequestError && err.code === 'version_conflict') {
        toast({ title: 'Template changed meanwhile', description: 'Reopen it to load the latest version.', variant: 'destructive' });
      }
    }
  };

  const localTitle = renderTemplateLocally(form.title_template, form.trigger);
  const localBody = renderTemplateLocally(form.body_template, form.trigger);
  const hasUnsavedSourceChanges =
    !!template &&
    (template.title_template !== form.title_template || template.body_template !== form.body_template || template.trigger !== form.trigger);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] max-w-4xl overflow-y-auto">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{template ? 'Edit template' : 'New notification template'}</DialogTitle>
            <DialogDescription>
              Titles and bodies are Go templates. Insert fields with the picker; unknown fields are rejected on save.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-6 lg:grid-cols-5">
            <div className="space-y-4 lg:col-span-3">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="tpl-name">Name *</Label>
                  <Input id="tpl-name" value={form.name} maxLength={255} onChange={(e) => set('name', e.target.value)} placeholder="e.g. Badge earned" />
                  {errors.name && <p className="text-xs text-destructive">{errors.name}</p>}
                </div>
                <div className="space-y-2">
                  <Label>Trigger *</Label>
                  <Select value={form.trigger} onValueChange={(v) => set('trigger', v as NotificationTrigger)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {NOTIFICATION_TRIGGERS.map((t) => (
                        <SelectItem key={t} value={t}>
                          {TRIGGER_LABELS[t]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {errors.trigger && <p className="text-xs text-destructive">{errors.trigger}</p>}
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-6">
                <div className="space-y-2">
                  <Label>Channels *</Label>
                  <div className="flex gap-4">
                    {NOTIFICATION_CHANNELS.map((ch) => (
                      <label key={ch} className="flex items-center gap-2 text-sm">
                        <Checkbox checked={form.channels.includes(ch)} onCheckedChange={(c) => toggleChannel(ch, c === true)} />
                        {CHANNEL_LABELS[ch]}
                      </label>
                    ))}
                  </div>
                  {errors.channels && <p className="text-xs text-destructive">{errors.channels}</p>}
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <Switch checked={form.is_active} onCheckedChange={(c) => set('is_active', c)} />
                  Active
                </label>
              </div>

              <div className="space-y-2">
                <Label htmlFor="tpl-title">Title template *</Label>
                <Input
                  id="tpl-title"
                  ref={titleRef}
                  className="font-mono text-sm"
                  value={form.title_template}
                  maxLength={500}
                  onFocus={() => setFocused('title_template')}
                  onChange={(e) => set('title_template', e.target.value)}
                />
                {errors.title_template && <p className="text-xs text-destructive font-mono">{errors.title_template}</p>}
              </div>

              <div className="space-y-2">
                <Label htmlFor="tpl-body">Body template</Label>
                <Textarea
                  id="tpl-body"
                  ref={bodyRef}
                  className="min-h-[120px] font-mono text-sm"
                  value={form.body_template}
                  maxLength={10000}
                  onFocus={() => setFocused('body_template')}
                  onChange={(e) => set('body_template', e.target.value)}
                />
                {errors.body_template && <p className="text-xs text-destructive font-mono">{errors.body_template}</p>}
              </div>

              <div className="space-y-2 rounded-lg border p-3">
                <p className="text-xs font-medium text-muted-foreground">
                  Variables for “{TRIGGER_LABELS[form.trigger]}” · inserted into the {focused === 'title_template' ? 'title' : 'body'}
                </p>
                {TRIGGER_VARIABLES[form.trigger].map((group) => (
                  <div key={group.label} className="flex flex-wrap items-center gap-1.5">
                    <span className="w-14 text-xs text-muted-foreground">{group.label}</span>
                    {group.variables.map((variable) => (
                      <button
                        key={variable.path}
                        type="button"
                        title={variable.description}
                        onClick={() => insertVariable(variable.path)}
                        className="rounded border bg-secondary/60 px-1.5 py-0.5 font-mono text-xs hover:border-primary/50 hover:bg-primary/10"
                      >
                        {variable.path}
                      </button>
                    ))}
                  </div>
                ))}
              </div>

              {errors.form && <p className="text-sm text-destructive">{errors.form}</p>}
            </div>

            <div className="space-y-3 lg:col-span-2">
              <Tabs defaultValue="live">
                <TabsList className="grid w-full grid-cols-2">
                  <TabsTrigger value="live">Live preview</TabsTrigger>
                  <TabsTrigger value="server" disabled={!template}>
                    Server render
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="live" className="space-y-2">
                  <PreviewBox title={localTitle} body={localBody} />
                  <p className="text-xs text-muted-foreground">
                    Approximate, with sample data. Conditionals are not evaluated here; the server render is exact.
                  </p>
                </TabsContent>
                <TabsContent value="server" className="space-y-3">
                  {template && (
                    <>
                      <div className="space-y-2">
                        <Label className="text-xs">Render for a player (optional)</Label>
                        <PlayerPicker value={previewPlayer} onChange={setPreviewPlayer} placeholder="Sample data, or search a player..." />
                      </div>
                      <Button type="button" variant="outline" size="sm" className="w-full gap-2" onClick={runServerPreview} disabled={previewMutation.isPending}>
                        {previewMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : previewMutation.data ? <RefreshCw className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                        Render saved template
                      </Button>
                      {hasUnsavedSourceChanges && (
                        <p className="text-xs text-amber-600 dark:text-amber-400">Renders the saved version; save to preview your edits.</p>
                      )}
                      {previewMutation.isError && <p className="text-xs text-destructive">{previewMutation.error.message}</p>}
                      {previewMutation.data && (
                        <>
                          <PreviewBox title={previewMutation.data.title} body={previewMutation.data.body} />
                          {form.channels.includes('email') && previewMutation.data.html && (
                            <div className="space-y-1">
                              <Badge variant="outline" className="text-xs">Email HTML</Badge>
                              <iframe
                                title="Email preview"
                                sandbox=""
                                srcDoc={previewMutation.data.html}
                                className="h-48 w-full rounded-md border bg-white"
                              />
                            </div>
                          )}
                        </>
                      )}
                    </>
                  )}
                </TabsContent>
              </Tabs>
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isSaving}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving && <Loader2 className="h-4 w-4 animate-spin" />}
              {template ? 'Save changes' : 'Create template'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function PreviewBox({ title, body }: { title: string; body: string }) {
  return (
    <div className="space-y-1 rounded-lg border bg-secondary/40 p-3">
      <p className="break-words text-sm font-semibold">{title || <span className="text-muted-foreground">(empty title)</span>}</p>
      <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">{body || '(no body)'}</p>
    </div>
  );
}
