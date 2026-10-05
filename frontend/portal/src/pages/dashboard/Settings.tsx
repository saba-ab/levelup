import React, { useEffect, useMemo, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { useAuth } from '@/contexts/AuthContext';
import ApiKeysSettings from '@/components/ApiKeysSettings';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import {
  UserPlus,
  MoreHorizontal,
  Pencil,
  Trash2,
  Shield,
  Crown,
  Code,
  Settings2,
  Users,
  Lock,
  UserX,
  UserCheck,
  Loader2,
  KeyRound,
} from 'lucide-react';
import { useAuthService } from '@/services/api/auth';
import {
  useUsersQuery,
  useCreateUserMutation,
  useUpdateUserMutation,
  useAssignUserRolesMutation,
  useDeleteUserMutation,
} from '@/services/queries/users';
import { ApiRequestError } from '@/services/queries/rules';
import { ROLE_IDS } from '@/services/api/types';
import type { RoleKey, Tenant, User } from '@/services/api/types';
import CursorPager from '@/components/CursorPager';

const roleConfig: Record<RoleKey, { label: string; color: string; icon: React.ReactNode; description: string }> = {
  owner: {
    label: 'Owner',
    color: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
    icon: <Crown className="h-3 w-3" />,
    description: 'Full access, including tenant deletion and ownership.',
  },
  super_admin: {
    label: 'Super Admin',
    color: 'bg-purple-500/20 text-purple-400 border-purple-500/30',
    icon: <Shield className="h-3 w-3" />,
    description: 'Full access to configuration and team management.',
  },
  admin: {
    label: 'Admin',
    color: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
    icon: <Settings2 className="h-3 w-3" />,
    description: 'Manage mechanics, players, rules and programs.',
  },
  program_manager: {
    label: 'Program Manager',
    color: 'bg-cyan-500/20 text-cyan-400 border-cyan-500/30',
    icon: <Users className="h-3 w-3" />,
    description: 'Create and manage gamification programs and rules.',
  },
  developer: {
    label: 'Developer',
    color: 'bg-orange-500/20 text-orange-400 border-orange-500/30',
    icon: <Code className="h-3 w-3" />,
    description: 'Integrate via the API: send activities and read data.',
  },
  platform_admin: {
    label: 'Platform Admin',
    color: 'bg-red-500/20 text-red-400 border-red-500/30',
    icon: <Shield className="h-3 w-3" />,
    description: 'LevelUp operator account (not assignable by tenants).',
  },
};

/** Roles a tenant can assign (owner is transferred, platform_admin is internal). */
const ASSIGNABLE_ROLES: RoleKey[] = ['super_admin', 'admin', 'program_manager', 'developer'];

function errorText(err: unknown, fallback: string): string {
  if (err instanceof ApiRequestError && err.validationErrors) {
    return Object.entries(err.validationErrors)
      .map(([f, m]) => `${f}: ${m.join(', ')}`)
      .join('\n');
  }
  return err instanceof Error ? err.message : fallback;
}

const primaryRole = (member: User): RoleKey | undefined => {
  const keys = member.roles.map(r => r.key);
  return (['owner', 'super_admin', 'admin', 'program_manager', 'developer', 'platform_admin'] as RoleKey[]).find(k =>
    keys.includes(k),
  );
};

function timezones(): string[] {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
  try {
    return intl.supportedValuesOf?.('timeZone') ?? ['UTC'];
  } catch {
    return ['UTC'];
  }
}

function GeneralSettings({ canManage }: { canManage: boolean }) {
  const { tenant: sessionTenant, applyTenant } = useAuth();
  const { getTenant, updateTenant } = useAuthService();
  const { toast } = useToast();
  const [tenant, setTenant] = useState<Tenant | null>(sessionTenant);
  const [name, setName] = useState(sessionTenant?.name ?? '');
  const [timezone, setTimezone] = useState(sessionTenant?.timezone ?? 'UTC');
  const [logoUrl, setLogoUrl] = useState(String(sessionTenant?.settings?.logo_url ?? ''));
  const [saving, setSaving] = useState(false);
  const zones = useMemo(timezones, []);

  useEffect(() => {
    let cancelled = false;
    getTenant().then(res => {
      if (cancelled || !res.success || !res.data) return;
      setTenant(res.data);
      setName(res.data.name);
      setTimezone(res.data.timezone);
      setLogoUrl(String(res.data.settings?.logo_url ?? ''));
    });
    return () => {
      cancelled = true;
    };
  }, [getTenant]);

  const save = async (section: 'general' | 'branding') => {
    if (!tenant) return;
    const patch: { name?: string; timezone?: string; settings?: Record<string, unknown> } = {};
    if (section === 'general') {
      if (name.trim() && name.trim() !== tenant.name) patch.name = name.trim();
      if (timezone !== tenant.timezone) patch.timezone = timezone;
    } else if (logoUrl.trim() !== String(tenant.settings?.logo_url ?? '')) {
      patch.settings = { ...tenant.settings, logo_url: logoUrl.trim() || undefined };
    }
    if (Object.keys(patch).length === 0) {
      toast({ title: 'Nothing to save' });
      return;
    }
    setSaving(true);
    const res = await updateTenant(patch);
    setSaving(false);
    if (res.success && res.data) {
      setTenant(res.data);
      applyTenant(res.data);
      toast({ title: 'Settings saved', description: 'Your tenant settings were updated.' });
    }
  };

  if (!tenant) {
    return (
      <Card>
        <CardContent className="p-6 space-y-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-1/2" />
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <TabsContent value="general" className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>General Settings</CardTitle>
            <CardDescription>Your tenant's name and timezone (streak periods are computed in it).</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Tenant Name</label>
                <Input value={name} onChange={e => setName(e.target.value)} disabled={!canManage} maxLength={255} />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Timezone</label>
                <Input
                  list="tenant-timezones"
                  value={timezone}
                  onChange={e => setTimezone(e.target.value)}
                  disabled={!canManage}
                />
                <datalist id="tenant-timezones">
                  {zones.map(z => (
                    <option key={z} value={z} />
                  ))}
                </datalist>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              Slug: <code>{tenant.slug}</code>
            </p>
            <Button variant="glow" onClick={() => save('general')} disabled={!canManage || saving}>
              {saving && <Loader2 className="h-4 w-4 animate-spin" />}
              Save Changes
            </Button>
          </CardContent>
        </Card>
      </TabsContent>

      <TabsContent value="branding">
        <Card>
          <CardHeader>
            <CardTitle>Branding</CardTitle>
            <CardDescription>Stored in your tenant settings.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Logo URL</label>
              <Input
                placeholder="https://example.com/logo.png"
                value={logoUrl}
                onChange={e => setLogoUrl(e.target.value)}
                disabled={!canManage}
              />
            </div>
            <Button variant="glow" onClick={() => save('branding')} disabled={!canManage || saving}>
              Save Changes
            </Button>
          </CardContent>
        </Card>
      </TabsContent>
    </>
  );
}

function TeamSettings() {
  const { user } = useAuth();
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const { data, isLoading, isFetching, error } = useUsersQuery({ limit: pager.limit, cursor: pager.cursor });
  const createUser = useCreateUserMutation();
  const updateUser = useUpdateUserMutation();
  const assignRoles = useAssignUserRolesMutation();
  const deleteUser = useDeleteUserMutation();

  const [addOpen, setAddOpen] = useState(false);
  const [addForm, setAddForm] = useState({ name: '', email: '', password: '', role: 'developer' as RoleKey });
  const [addError, setAddError] = useState<string | null>(null);
  const [editMember, setEditMember] = useState<User | null>(null);
  const [editRole, setEditRole] = useState<RoleKey>('developer');
  const [removeMember, setRemoveMember] = useState<User | null>(null);
  const [passwordMember, setPasswordMember] = useState<User | null>(null);
  const [newPassword, setNewPassword] = useState('');

  const members = data?.data ?? [];

  const handleAdd = async () => {
    setAddError(null);
    if (!addForm.name.trim() || !addForm.email.trim() || addForm.password.length < 8) {
      setAddError('Name, email and a password of at least 8 characters are required.');
      return;
    }
    try {
      await createUser.mutateAsync({
        name: addForm.name.trim(),
        email: addForm.email.trim(),
        password: addForm.password,
        role_ids: [ROLE_IDS[addForm.role]],
      });
      toast({ title: 'Member added', description: `${addForm.name} can now sign in as ${roleConfig[addForm.role].label}.` });
      setAddOpen(false);
      setAddForm({ name: '', email: '', password: '', role: 'developer' });
    } catch (err) {
      setAddError(errorText(err, 'Failed to add member'));
    }
  };

  const handleRoleSave = async () => {
    if (!editMember) return;
    try {
      await assignRoles.mutateAsync({ userId: editMember.id, roleIds: [ROLE_IDS[editRole]] });
      toast({ title: 'Role updated', description: `${editMember.name} is now ${roleConfig[editRole].label}.` });
      setEditMember(null);
    } catch (err) {
      toast({ title: 'Error', description: errorText(err, 'Failed to update role'), variant: 'destructive' });
    }
  };

  const toggleActive = async (member: User) => {
    try {
      await updateUser.mutateAsync({ userId: member.id, data: { active: !member.active } });
      toast({ title: member.active ? 'Member deactivated' : 'Member reactivated' });
    } catch (err) {
      toast({ title: 'Error', description: errorText(err, 'Failed to update member'), variant: 'destructive' });
    }
  };

  const handleSetPassword = async () => {
    if (!passwordMember || newPassword.length < 8) return;
    try {
      await updateUser.mutateAsync({ userId: passwordMember.id, data: { password: newPassword } });
      toast({ title: 'Password updated', description: `Share the new password with ${passwordMember.name} securely.` });
      setPasswordMember(null);
      setNewPassword('');
    } catch (err) {
      toast({ title: 'Error', description: errorText(err, 'Failed to set password'), variant: 'destructive' });
    }
  };

  const handleRemove = async () => {
    if (!removeMember) return;
    const member = removeMember;
    setRemoveMember(null);
    try {
      await deleteUser.mutateAsync(member.id);
      toast({ title: 'Member removed', description: `${member.name} no longer has access.` });
    } catch (err) {
      toast({ title: 'Error', description: errorText(err, 'Failed to remove member'), variant: 'destructive' });
    }
  };

  return (
    <TabsContent value="team" className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Team Members</CardTitle>
            <CardDescription>Manage who has access to this tenant.</CardDescription>
          </div>
          <Button variant="glow" size="sm" onClick={() => setAddOpen(true)}>
            <UserPlus className="h-4 w-4 mr-2" />
            Add Member
          </Button>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Member</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Added</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-[50px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={5}>
                    <Skeleton className="h-8 w-full" />
                  </TableCell>
                </TableRow>
              ) : error ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-destructive text-center">
                    {error.message}
                  </TableCell>
                </TableRow>
              ) : (
                members.map(member => {
                  const role = primaryRole(member);
                  const cfg = role ? roleConfig[role] : undefined;
                  const isSelf = member.id === user?.id;
                  const isOwner = member.roles.some(r => r.key === 'owner');
                  return (
                    <TableRow key={member.id}>
                      <TableCell>
                        <div>
                          <p className="font-medium">
                            {member.name}
                            {isSelf && <span className="text-xs text-muted-foreground ml-2">(you)</span>}
                          </p>
                          <p className="text-sm text-muted-foreground">{member.email}</p>
                        </div>
                      </TableCell>
                      <TableCell>
                        {cfg ? (
                          <Badge variant="outline" className={`${cfg.color} gap-1`}>
                            {cfg.icon}
                            {cfg.label}
                          </Badge>
                        ) : (
                          <Badge variant="outline">No role</Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(member.created_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell>
                        <Badge variant={member.active ? 'default' : 'secondary'}>
                          {member.active ? 'active' : 'inactive'}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {!isOwner && !isSelf && (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon" className="h-8 w-8">
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem
                                onClick={() => {
                                  setEditMember(member);
                                  setEditRole(role && ASSIGNABLE_ROLES.includes(role) ? role : 'developer');
                                }}
                              >
                                <Pencil className="h-4 w-4 mr-2" />
                                Edit Role
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => setPasswordMember(member)}>
                                <KeyRound className="h-4 w-4 mr-2" />
                                Set Password
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => toggleActive(member)}>
                                {member.active ? (
                                  <>
                                    <UserX className="h-4 w-4 mr-2" />
                                    Deactivate
                                  </>
                                ) : (
                                  <>
                                    <UserCheck className="h-4 w-4 mr-2" />
                                    Reactivate
                                  </>
                                )}
                              </DropdownMenuItem>
                              <DropdownMenuItem className="text-destructive" onClick={() => setRemoveMember(member)}>
                                <Trash2 className="h-4 w-4 mr-2" />
                                Remove
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={data?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Role Permissions</CardTitle>
          <CardDescription>Overview of what each role can do.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {(['owner', ...ASSIGNABLE_ROLES] as RoleKey[]).map(role => {
              const config = roleConfig[role];
              return (
                <div key={role} className="p-4 rounded-lg border bg-card/50">
                  <div className="flex items-center gap-2 mb-2">
                    <Badge variant="outline" className={`${config.color} gap-1`}>
                      {config.icon}
                      {config.label}
                    </Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">{config.description}</p>
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>

      {/* Add member */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Team Member</DialogTitle>
            <DialogDescription>
              Creates an account with a temporary password. Share it securely; they can change it after signing in.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Name</label>
              <Input value={addForm.name} onChange={e => setAddForm(f => ({ ...f, name: e.target.value }))} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Email</label>
              <Input type="email" value={addForm.email} onChange={e => setAddForm(f => ({ ...f, email: e.target.value }))} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Temporary password</label>
              <Input
                type="password"
                autoComplete="new-password"
                value={addForm.password}
                onChange={e => setAddForm(f => ({ ...f, password: e.target.value }))}
                placeholder="At least 8 characters"
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Role</label>
              <Select value={addForm.role} onValueChange={v => setAddForm(f => ({ ...f, role: v as RoleKey }))}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ASSIGNABLE_ROLES.map(r => (
                    <SelectItem key={r} value={r}>
                      {roleConfig[r].label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {addError && <p className="text-sm text-destructive whitespace-pre-line">{addError}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button variant="glow" onClick={handleAdd} disabled={createUser.isPending}>
              {createUser.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              Add Member
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit role */}
      <Dialog open={!!editMember} onOpenChange={o => !o && setEditMember(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Role for {editMember?.name}</DialogTitle>
          </DialogHeader>
          <div className="space-y-2 py-4">
            <label className="text-sm font-medium">Role</label>
            <Select value={editRole} onValueChange={v => setEditRole(v as RoleKey)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ASSIGNABLE_ROLES.map(r => (
                  <SelectItem key={r} value={r}>
                    {roleConfig[r].label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditMember(null)}>
              Cancel
            </Button>
            <Button variant="glow" onClick={handleRoleSave} disabled={assignRoles.isPending}>
              Save Role
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Set password */}
      <Dialog
        open={!!passwordMember}
        onOpenChange={o => {
          if (!o) {
            setPasswordMember(null);
            setNewPassword('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Set a new password for {passwordMember?.name}</DialogTitle>
            <DialogDescription>They use it the next time they sign in.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-4">
            <label className="text-sm font-medium">New password</label>
            <Input
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={e => setNewPassword(e.target.value)}
              placeholder="At least 8 characters"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPasswordMember(null)}>
              Cancel
            </Button>
            <Button variant="glow" onClick={handleSetPassword} disabled={newPassword.length < 8 || updateUser.isPending}>
              Set Password
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Remove */}
      <AlertDialog open={!!removeMember} onOpenChange={o => !o && setRemoveMember(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove {removeMember?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              They lose access to this tenant immediately. Deactivating keeps the account if you may need it again.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleRemove}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </TabsContent>
  );
}

export default function Settings() {
  const { hasPermission } = useAuth();
  const canManageTeam = hasPermission('manage:team');
  const canManageSettings = hasPermission('manage:settings');

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-3xl font-bold">Settings</h1>
        <p className="text-muted-foreground mt-1">Manage your tenant and team.</p>
      </div>

      <Tabs defaultValue="general" className="space-y-6">
        <TabsList>
          <TabsTrigger value="general">General</TabsTrigger>
          <TabsTrigger value="branding">Branding</TabsTrigger>
          <TabsTrigger value="team" disabled={!canManageTeam} className="gap-2">
            Team
            {!canManageTeam && <Lock className="h-3 w-3" />}
          </TabsTrigger>
          <TabsTrigger value="api-keys" disabled={!canManageTeam} className="gap-2">
            API Keys
            {!canManageTeam && <Lock className="h-3 w-3" />}
          </TabsTrigger>
        </TabsList>

        <GeneralSettings canManage={canManageSettings} />
        {canManageTeam && <TeamSettings />}
        {canManageTeam && (
          <TabsContent value="api-keys">
            <ApiKeysSettings />
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
