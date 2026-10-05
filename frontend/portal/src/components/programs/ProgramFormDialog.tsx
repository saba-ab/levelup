import React, { useState, useEffect } from 'react';
import { format } from 'date-fns';
import { CalendarIcon } from 'lucide-react';
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
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Separator } from '@/components/ui/separator';
import { Calendar } from '@/components/ui/calendar';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import type { Program, CreateProgramData, UpdateProgramData, ProgramSettings, ProgramMechanics } from '@/services/api/types';

interface ProgramFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  program?: Program | null;
  /**
   * Create: the full CreateProgramData. Edit: a partial PATCH body with only
   * the changed fields (cleared description/dates are sent as null). Not
   * called when nothing changed; the dialog just closes.
   */
  onSubmit: (data: CreateProgramData | UpdateProgramData) => void;
  isLoading?: boolean;
}

const MECHANIC_TOGGLES: { key: keyof ProgramMechanics & string; label: string }[] = [
  { key: 'points_enabled', label: 'Points' },
  { key: 'badges_enabled', label: 'Badges' },
  { key: 'levels_enabled', label: 'Levels' },
  { key: 'missions_enabled', label: 'Missions' },
  { key: 'streaks_enabled', label: 'Streaks' },
  { key: 'leaderboards_enabled', label: 'Leaderboards' },
  { key: 'rewards_enabled', label: 'Rewards' },
];

const sameJson = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

const defaultSettings: ProgramSettings = {
  allow_public_signup: false,
  require_email_verification: true,
  welcome_points: 0,
};

const defaultMechanics: ProgramMechanics = {
  points_enabled: true,
  badges_enabled: true,
  levels_enabled: true,
  missions_enabled: true,
  streaks_enabled: true,
  leaderboards_enabled: true,
  rewards_enabled: true,
};

export function ProgramFormDialog({
  open,
  onOpenChange,
  program,
  onSubmit,
  isLoading,
}: ProgramFormDialogProps) {
  const isEditing = !!program;

  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [startDate, setStartDate] = useState<Date | undefined>();
  const [endDate, setEndDate] = useState<Date | undefined>();
  const [settings, setSettings] = useState<ProgramSettings>(defaultSettings);
  const [mechanics, setMechanics] = useState<ProgramMechanics>(defaultMechanics);

  // Reset form when dialog opens/closes or program changes
  useEffect(() => {
    if (open) {
      if (program) {
        setName(program.name);
        setSlug(program.slug);
        setDescription(program.description || '');
        setStartDate(program.starts_at ? new Date(program.starts_at) : undefined);
        setEndDate(program.ends_at ? new Date(program.ends_at) : undefined);
        setSettings({ ...defaultSettings, ...program.settings });
        setMechanics({ ...defaultMechanics, ...program.mechanics });
      } else {
        setName('');
        setSlug('');
        setDescription('');
        setStartDate(undefined);
        setEndDate(undefined);
        setSettings(defaultSettings);
        setMechanics(defaultMechanics);
      }
    }
  }, [open, program]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = name.trim();
    const trimmedSlug = slug.trim();
    const trimmedDescription = description.trim();
    const startsAt = startDate ? startDate.toISOString() : null;
    const endsAt = endDate ? endDate.toISOString() : null;

    if (!program) {
      const data: CreateProgramData = {
        name: trimmedName,
        ...(trimmedSlug && { slug: trimmedSlug }),
        ...(trimmedDescription && { description: trimmedDescription }),
        ...(startsAt && { starts_at: startsAt }),
        ...(endsAt && { ends_at: endsAt }),
        settings,
        mechanics,
      };
      onSubmit(data);
      return;
    }

    const sameInstant = (a: string | null, b: string | null) =>
      (a === null ? null : new Date(a).getTime()) === (b === null ? null : new Date(b).getTime());

    const patch: UpdateProgramData = {};
    if (trimmedName !== program.name) patch.name = trimmedName;
    if (trimmedSlug && trimmedSlug !== program.slug) patch.slug = trimmedSlug;
    if (trimmedDescription !== (program.description ?? '')) patch.description = trimmedDescription || null;
    if (!sameInstant(startsAt, program.starts_at)) patch.starts_at = startsAt;
    if (!sameInstant(endsAt, program.ends_at)) patch.ends_at = endsAt;
    if (!sameJson(settings, { ...defaultSettings, ...program.settings })) patch.settings = settings;
    if (!sameJson(mechanics, { ...defaultMechanics, ...program.mechanics })) patch.mechanics = mechanics;

    if (Object.keys(patch).length === 0) {
      onOpenChange(false);
      return;
    }
    onSubmit(patch);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{isEditing ? 'Edit Program' : 'Create New Program'}</DialogTitle>
            <DialogDescription>
              {isEditing
                ? 'Update your program settings and mechanics.'
                : 'Add a new gamification program to organize your rules and mechanics.'}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-6 py-4">
            {/* Basic Info */}
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">Program Name *</Label>
                <Input
                  id="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g., Loyalty Rewards"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="slug">Slug</Label>
                <Input
                  id="slug"
                  value={slug}
                  onChange={(e) => setSlug(e.target.value)}
                  placeholder={isEditing ? undefined : 'Generated from the name when left empty'}
                  maxLength={120}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="description">Description</Label>
                <Textarea
                  id="description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Describe your program..."
                  rows={3}
                  maxLength={1000}
                />
              </div>
            </div>

            {/* Date Range */}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>Start Date</Label>
                  {startDate && (
                    <Button type="button" variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={() => setStartDate(undefined)}>
                      Clear
                    </Button>
                  )}
                </div>
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      className={cn(
                        'w-full justify-start text-left font-normal',
                        !startDate && 'text-muted-foreground'
                      )}
                    >
                      <CalendarIcon className="mr-2 h-4 w-4" />
                      {startDate ? format(startDate, 'PPP') : 'Pick a date'}
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-auto p-0" align="start">
                    <Calendar
                      mode="single"
                      selected={startDate}
                      onSelect={setStartDate}
                      initialFocus
                      className="p-3 pointer-events-auto"
                    />
                  </PopoverContent>
                </Popover>
              </div>
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label>End Date</Label>
                  {endDate && (
                    <Button type="button" variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={() => setEndDate(undefined)}>
                      Clear
                    </Button>
                  )}
                </div>
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      className={cn(
                        'w-full justify-start text-left font-normal',
                        !endDate && 'text-muted-foreground'
                      )}
                    >
                      <CalendarIcon className="mr-2 h-4 w-4" />
                      {endDate ? format(endDate, 'PPP') : 'Pick a date'}
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-auto p-0" align="start">
                    <Calendar
                      mode="single"
                      selected={endDate}
                      onSelect={setEndDate}
                      initialFocus
                      className="p-3 pointer-events-auto"
                    />
                  </PopoverContent>
                </Popover>
              </div>
            </div>

            <Separator />

            {/* Settings */}
            <div className="space-y-4">
              <h4 className="text-sm font-medium">Settings</h4>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label>Allow Public Signup</Label>
                    <p className="text-sm text-muted-foreground">
                      Players can join this program without invitation
                    </p>
                  </div>
                  <Switch
                    checked={settings.allow_public_signup ?? false}
                    onCheckedChange={(checked) =>
                      setSettings({ ...settings, allow_public_signup: checked })
                    }
                  />
                </div>
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label>Require Email Verification</Label>
                    <p className="text-sm text-muted-foreground">
                      Players must verify their email to participate
                    </p>
                  </div>
                  <Switch
                    checked={settings.require_email_verification ?? false}
                    onCheckedChange={(checked) =>
                      setSettings({ ...settings, require_email_verification: checked })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="welcome_points">Welcome Points</Label>
                  <Input
                    id="welcome_points"
                    type="number"
                    min={0}
                    value={settings.welcome_points ?? 0}
                    onChange={(e) =>
                      setSettings({ ...settings, welcome_points: Math.max(0, Math.floor(Number(e.target.value) || 0)) })
                    }
                    placeholder="0"
                  />
                  <p className="text-sm text-muted-foreground">
                    Points awarded when a player joins the program
                  </p>
                </div>
              </div>
            </div>

            <Separator />

            {/* Mechanics */}
            <div className="space-y-4">
              <h4 className="text-sm font-medium">Enabled Mechanics</h4>
              <div className="grid grid-cols-2 gap-4">
                {MECHANIC_TOGGLES.map(({ key, label }) => (
                  <div key={key} className="flex items-center justify-between">
                    <Label>{label}</Label>
                    <Switch
                      checked={Boolean(mechanics[key])}
                      onCheckedChange={(checked) =>
                        setMechanics({ ...mechanics, [key]: checked })
                      }
                    />
                  </div>
                ))}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isLoading}
            >
              Cancel
            </Button>
            <Button type="submit" variant="glow" disabled={isLoading || !name.trim()}>
              {isLoading ? 'Saving...' : isEditing ? 'Save Changes' : 'Create Program'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
