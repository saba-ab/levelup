import React, { useState } from 'react';
import { Search, Users as UsersIcon } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { users } from '@/lib/mockData';
import { cn } from '@/lib/utils';

export default function Users() {
  const [searchQuery, setSearchQuery] = useState('');

  const filteredUsers = users.filter(user =>
    user.email.toLowerCase().includes(searchQuery.toLowerCase()) ||
    user.id.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const getLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'diamond': return 'border-cyan-400 text-cyan-400 bg-cyan-400/10';
      case 'platinum': return 'border-slate-300 text-slate-300 bg-slate-300/10';
      case 'gold': return 'border-yellow-500 text-yellow-500 bg-yellow-500/10';
      case 'silver': return 'border-gray-400 text-gray-400 bg-gray-400/10';
      default: return 'border-amber-700 text-amber-700 bg-amber-700/10';
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-3xl font-bold">Users</h1>
        <p className="text-muted-foreground mt-1">View and manage your platform users.</p>
      </div>

      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input placeholder="Search by email or user ID..." className="pl-9" value={searchQuery} onChange={(e) => setSearchQuery(e.target.value)} />
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">User</th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Level</th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Total XP</th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Last Active</th>
              </tr>
            </thead>
            <tbody>
              {filteredUsers.map((user) => (
                <tr key={user.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors cursor-pointer">
                  <td className="p-4">
                    <div className="flex items-center gap-3">
                      <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
                        <span className="text-sm font-medium">{user.email[0].toUpperCase()}</span>
                      </div>
                      <div>
                        <p className="font-medium">{user.email}</p>
                        <p className="text-xs text-muted-foreground">{user.id}</p>
                      </div>
                    </div>
                  </td>
                  <td className="p-4">
                    <Badge variant="outline" className={cn("capitalize", getLevelColor(user.level))}>{user.level}</Badge>
                  </td>
                  <td className="p-4 font-mono">{user.totalXp.toLocaleString()}</td>
                  <td className="p-4 text-muted-foreground">{user.lastActive}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
