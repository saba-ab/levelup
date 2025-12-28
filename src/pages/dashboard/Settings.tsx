import React, { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from '@/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { useAuth } from '@/contexts/AuthContext';
import { useToast } from '@/hooks/use-toast';
import { UserPlus, MoreHorizontal, Pencil, Trash2, Shield, Crown, Code, BarChart3, Settings2, Users } from 'lucide-react';

type TeamRole = 'owner' | 'super_admin' | 'admin' | 'analyst' | 'program_manager' | 'developer';

interface TeamMember {
  id: string;
  email: string;
  name: string;
  role: TeamRole;
  invitedAt: string;
  status: 'active' | 'pending';
}

const roleConfig: Record<TeamRole, { label: string; color: string; icon: React.ReactNode }> = {
  owner: { label: 'Owner', color: 'bg-amber-500/20 text-amber-400 border-amber-500/30', icon: <Crown className="h-3 w-3" /> },
  super_admin: { label: 'Super Admin', color: 'bg-purple-500/20 text-purple-400 border-purple-500/30', icon: <Shield className="h-3 w-3" /> },
  admin: { label: 'Admin', color: 'bg-blue-500/20 text-blue-400 border-blue-500/30', icon: <Settings2 className="h-3 w-3" /> },
  analyst: { label: 'Analyst', color: 'bg-green-500/20 text-green-400 border-green-500/30', icon: <BarChart3 className="h-3 w-3" /> },
  program_manager: { label: 'Program Manager', color: 'bg-cyan-500/20 text-cyan-400 border-cyan-500/30', icon: <Users className="h-3 w-3" /> },
  developer: { label: 'Developer', color: 'bg-orange-500/20 text-orange-400 border-orange-500/30', icon: <Code className="h-3 w-3" /> },
};

const initialTeamMembers: TeamMember[] = [
  { id: '1', email: 'owner@levelupos.com', name: 'John Owner', role: 'owner', invitedAt: '2024-01-15', status: 'active' },
  { id: '2', email: 'admin@levelupos.com', name: 'Sarah Admin', role: 'super_admin', invitedAt: '2024-02-10', status: 'active' },
  { id: '3', email: 'dev@levelupos.com', name: 'Mike Developer', role: 'developer', invitedAt: '2024-03-05', status: 'active' },
  { id: '4', email: 'analyst@levelupos.com', name: 'Emma Analyst', role: 'analyst', invitedAt: '2024-03-20', status: 'pending' },
];

export default function Settings() {
  const { user } = useAuth();
  const { toast } = useToast();
  const [teamMembers, setTeamMembers] = useState<TeamMember[]>(initialTeamMembers);
  const [inviteDialogOpen, setInviteDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedMember, setSelectedMember] = useState<TeamMember | null>(null);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteName, setInviteName] = useState('');
  const [inviteRole, setInviteRole] = useState<TeamRole>('developer');

  const handleSave = () => {
    toast({ title: 'Settings saved', description: 'Your changes have been saved successfully.' });
  };

  const handleInviteMember = () => {
    if (!inviteEmail || !inviteName) {
      toast({ title: 'Error', description: 'Please fill in all fields.', variant: 'destructive' });
      return;
    }
    const newMember: TeamMember = {
      id: Date.now().toString(),
      email: inviteEmail,
      name: inviteName,
      role: inviteRole,
      invitedAt: new Date().toISOString().split('T')[0],
      status: 'pending',
    };
    setTeamMembers([...teamMembers, newMember]);
    setInviteDialogOpen(false);
    setInviteEmail('');
    setInviteName('');
    setInviteRole('developer');
    toast({ title: 'Invitation sent', description: `${inviteName} has been invited as ${roleConfig[inviteRole].label}.` });
  };

  const handleUpdateRole = (memberId: string, newRole: TeamRole) => {
    setTeamMembers(teamMembers.map(m => m.id === memberId ? { ...m, role: newRole } : m));
    setEditDialogOpen(false);
    setSelectedMember(null);
    toast({ title: 'Role updated', description: 'Team member role has been updated.' });
  };

  const handleRemoveMember = (memberId: string) => {
    const member = teamMembers.find(m => m.id === memberId);
    if (member?.role === 'owner') {
      toast({ title: 'Cannot remove owner', description: 'The owner cannot be removed.', variant: 'destructive' });
      return;
    }
    setTeamMembers(teamMembers.filter(m => m.id !== memberId));
    toast({ title: 'Member removed', description: 'Team member has been removed.' });
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-3xl font-bold">Settings</h1>
        <p className="text-muted-foreground mt-1">Manage your tenant and account settings.</p>
      </div>

      <Tabs defaultValue="general" className="space-y-6">
        <TabsList>
          <TabsTrigger value="general">General</TabsTrigger>
          <TabsTrigger value="branding">Branding</TabsTrigger>
          <TabsTrigger value="team">Team</TabsTrigger>
        </TabsList>

        <TabsContent value="general" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>General Settings</CardTitle>
              <CardDescription>Configure your tenant's basic information.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">Tenant Name</label>
                  <Input defaultValue={user?.tenantName} />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Default Timezone</label>
                  <Select defaultValue="UTC">
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="UTC">UTC</SelectItem>
                      <SelectItem value="America/New_York">Eastern Time</SelectItem>
                      <SelectItem value="Europe/London">London</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <Button variant="glow" onClick={handleSave}>Save Changes</Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="branding">
          <Card>
            <CardHeader>
              <CardTitle>Branding</CardTitle>
              <CardDescription>Customize the look and feel of your platform.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Logo URL</label>
                <Input placeholder="https://example.com/logo.png" />
              </div>
              <Button variant="glow" onClick={handleSave}>Save Changes</Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="team" className="space-y-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>Team Members</CardTitle>
                <CardDescription>Manage who has access to this tenant.</CardDescription>
              </div>
              <Dialog open={inviteDialogOpen} onOpenChange={setInviteDialogOpen}>
                <DialogTrigger asChild>
                  <Button variant="glow" size="sm">
                    <UserPlus className="h-4 w-4 mr-2" />
                    Invite Member
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Invite Team Member</DialogTitle>
                  </DialogHeader>
                  <div className="space-y-4 py-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Name</label>
                      <Input 
                        placeholder="John Doe" 
                        value={inviteName}
                        onChange={(e) => setInviteName(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Email</label>
                      <Input 
                        type="email" 
                        placeholder="john@example.com" 
                        value={inviteEmail}
                        onChange={(e) => setInviteEmail(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Role</label>
                      <Select value={inviteRole} onValueChange={(v) => setInviteRole(v as TeamRole)}>
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="super_admin">Super Admin</SelectItem>
                          <SelectItem value="admin">Admin</SelectItem>
                          <SelectItem value="analyst">Analyst</SelectItem>
                          <SelectItem value="program_manager">Program Manager</SelectItem>
                          <SelectItem value="developer">Developer</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <DialogFooter>
                    <Button variant="outline" onClick={() => setInviteDialogOpen(false)}>Cancel</Button>
                    <Button variant="glow" onClick={handleInviteMember}>Send Invitation</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Member</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Invited</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="w-[50px]">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {teamMembers.map((member) => (
                    <TableRow key={member.id}>
                      <TableCell>
                        <div>
                          <p className="font-medium">{member.name}</p>
                          <p className="text-sm text-muted-foreground">{member.email}</p>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`${roleConfig[member.role].color} gap-1`}>
                          {roleConfig[member.role].icon}
                          {roleConfig[member.role].label}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">{member.invitedAt}</TableCell>
                      <TableCell>
                        <Badge variant={member.status === 'active' ? 'default' : 'secondary'}>
                          {member.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {member.role !== 'owner' && (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon" className="h-8 w-8">
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem onClick={() => { setSelectedMember(member); setEditDialogOpen(true); }}>
                                <Pencil className="h-4 w-4 mr-2" />
                                Edit Role
                              </DropdownMenuItem>
                              <DropdownMenuItem 
                                className="text-destructive"
                                onClick={() => handleRemoveMember(member.id)}
                              >
                                <Trash2 className="h-4 w-4 mr-2" />
                                Remove
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Role Permissions</CardTitle>
              <CardDescription>Overview of what each role can do.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {Object.entries(roleConfig).map(([role, config]) => (
                  <div key={role} className="p-4 rounded-lg border bg-card/50">
                    <div className="flex items-center gap-2 mb-2">
                      <Badge variant="outline" className={`${config.color} gap-1`}>
                        {config.icon}
                        {config.label}
                      </Badge>
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {role === 'owner' && 'Full access to all features and billing.'}
                      {role === 'super_admin' && 'Full access except billing and ownership transfer.'}
                      {role === 'admin' && 'Manage mechanics, players, and programs.'}
                      {role === 'analyst' && 'View analytics and generate reports.'}
                      {role === 'program_manager' && 'Create and manage gamification programs.'}
                      {role === 'developer' && 'Access APIs, integrations, and documentation.'}
                    </p>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Edit Role Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Role for {selectedMember?.name}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">New Role</label>
              <Select 
                defaultValue={selectedMember?.role} 
                onValueChange={(v) => selectedMember && handleUpdateRole(selectedMember.id, v as TeamRole)}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="super_admin">Super Admin</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                  <SelectItem value="analyst">Analyst</SelectItem>
                  <SelectItem value="program_manager">Program Manager</SelectItem>
                  <SelectItem value="developer">Developer</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}