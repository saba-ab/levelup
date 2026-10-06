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
  Mail,
  Send,
  Ban,
} from 'lucide-react';
import { useAuthService } from '@/services/api/auth';
import {
  useUsersQuery,
  useCreateUserMutation,
  useUpdateUserMutation,
  useAssignUserRolesMutation,
  useDeleteUserMutation,
  useInvitationsQuery,
  useCreateInvitationMutation,
  useRevokeInvitationMutation,
} from '@/services/queries/users';
import type { Invitation } from '@/services/api/models/identity';
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

function TeamInvitations({ inviteOpen, setInviteOpen }: { inviteOpen: boolean; setInviteOpen: (open: boolean) => void }) {
  const { toast } = useToast();
  const pager = useCursorPagination(10);
  const { data, isLoading, isFetching, error, refetch } = useInvitationsQuery({ limit: pager.limit, cursor: pager.cursor });
  const createInvitation = useCreateInvitationMutation();
  const revokeInvitation = useRevokeInvitationMutation();
  const [form, setForm] = useState({ email: '', name: '', role: 'developer' as RoleKey });
  const [formError, setFormError] = useState<string | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<Invitation | null>(null);

  const invitations = data?.data ?? [];

  const closeInvite = (open: boolean) => {
    setInviteOpen(open);
    if (!open) {
      setForm({ email: '', name: '', role: 'developer' });
      setFormError(null);
    }
  };

  const handleInvite = async () => {
    setFormError(null);
    const email = form.email.trim();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      setFormError('Please enter a valid email address.');
      return;
    }
    try {
      await createInvitation.mutateAsync({
        email,
        name: form.name.trim() || undefined,
        role_ids: [ROLE_IDS[form.role]],
      });
      pager.reset();
      toast({ title: 'Invitation sent', description: `${email} will get an email with a link to join (valid 7 days).` });
      closeInvite(false);
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === 'email_taken') {
        setFormError('Someone with this email already has an account.');
        return;
      }
      setFormError(errorText(err, 'Failed to send invitation'));
    }
  };

  const handleResend = async (invitation: Invitation) => {
    const role = invitation.roles.map(r => r.key).find(k => ASSIGNABLE_ROLES.includes(k));
    try {
      await createInvitation.mutateAsync({
        email: invitation.email,
        name: invitation.name || undefined,
        role_ids: invitation.role_ids.length > 0 ? invitation.role_ids : [ROLE_IDS[role ?? 'developer']],
      });
      toast({ title: 'Invitation resent', description: `A new link was emailed to ${invitation.email}; the old one no longer works.` });
    } catch (err) {
      toast({ title: 'Error', description: errorText(err, 'Failed to resend invitation'), variant: 'destructive' });
    }
  };

  const handleRevoke = async () => {
    if (!revokeTarget) return;
    const invitation = revokeTarget;
    setRevokeTarget(null);
    try {
      await revokeInvitation.mutateAsync(invitation.id);
      toast({ title: 'Invitation revoked', description: `The link sent to ${invitation.email} no longer works.` });
    } catch (err) {
      toast({ title: 'Error', description: errorText(err, 'Failed to revoke invitation'), variant: 'destructive' });
    }
  };

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Pending Invitations</CardTitle>
            <CardDescription>People invited by email who haven't joined yet. Links are valid for 7 days.</CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={() => setInviteOpen(true)}>
            <Mail className="h-4 w-4 mr-2" />
            Invite by Email
          </Button>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Invitee</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Invited</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-[50px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Skeleton className="h-8 w-full" />
                  </TableCell>
                </TableRow>
              ) : error ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center">
                    <p className="text-destructive">{error.message}</p>
                    <Button variant="link" size="sm" onClick={() => refetch()}>
                      Retry
                    </Button>
                  </TableCell>
                </TableRow>
              ) : invitations.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-8">
                    No pending invitations. Invite teammates by email and they'll set their own password.
                  </TableCell>
                </TableRow>
              ) : (
                invitations.map(invitation => {
                  const role = invitation.roles[0]?.key;
                  const cfg = role ? roleConfig[role] : undefined;
                  const expired = invitation.status === 'expired';
                  return (
                    <TableRow key={invitation.id}>
                      <TableCell>
                        <div>
                          <p className="font-medium">{invitation.name || invitation.email}</p>
                          {invitation.name && <p className="text-sm text-muted-foreground">{invitation.email}</p>}
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
                        {new Date(invitation.created_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(invitation.expires_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell>
                        <Badge variant={expired ? 'secondary' : 'default'}>{invitation.status}</Badge>
                      </TableCell>
                      <TableCell>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" className="h-8 w-8">
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onClick={() => handleResend(invitation)} disabled={createInvitation.isPending}>
                              <Send className="h-4 w-4 mr-2" />
                              {expired ? 'Send new invitation' : 'Resend invitation'}
                            </DropdownMenuItem>
                            <DropdownMenuItem className="text-destructive" onClick={() => setRevokeTarget(invitation)}>
                              <Ban className="h-4 w-4 mr-2" />
                              Revoke
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
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

      {/* Invite by email */}
      <Dialog open={inviteOpen} onOpenChange={closeInvite}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Invite by Email</DialogTitle>
            <DialogDescription>
              We email them a link to join this tenant and choose their own password. Re-inviting an address replaces its
              earlier invitation.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Email</label>
              <Input
                type="email"
                placeholder="teammate@company.com"
                value={form.email}
                onChange={e => setForm(f => ({ ...f, email: e.target.value }))}
                maxLength={255}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">
                Name <span className="text-muted-foreground font-normal">(optional)</span>
              </label>
              <Input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} maxLength={255} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Role</label>
              <Select value={form.role} onValueChange={v => setForm(f => ({ ...f, role: v as RoleKey }))}>
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
              <p className="text-xs text-muted-foreground">{roleConfig[form.role].description}</p>
            </div>
            {formError && <p className="text-sm text-destructive whitespace-pre-line">{formError}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => closeInvite(false)}>
              Cancel
            </Button>
            <Button variant="glow" onClick={handleInvite} disabled={createInvitation.isPending}>
              {createInvitation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              Send Invitation
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Revoke */}
      <AlertDialog open={!!revokeTarget} onOpenChange={o => !o && setRevokeTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke invitation for {revokeTarget?.email}?</AlertDialogTitle>
            <AlertDialogDescription>
              The emailed link stops working immediately. You can invite them again later.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleRevoke}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Revoke
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
  const [inviteOpen, setInviteOpen] = useState(false);
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
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => setInviteOpen(true)}>
              <Mail className="h-4 w-4 mr-2" />
              Invite by Email
            </Button>
            <Button variant="glow" size="sm" onClick={() => setAddOpen(true)}>
              <UserPlus className="h-4 w-4 mr-2" />
              Add Member
            </Button>
          </div>
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

      <TeamInvitations inviteOpen={inviteOpen} setInviteOpen={setInviteOpen} />

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
