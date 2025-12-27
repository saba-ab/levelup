import React, { useState } from 'react';
import { Code, ChevronRight, Copy, Check, Lock, Unlock } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';

const endpoints = [
  {
    category: 'Points',
    items: [
      { method: 'POST', path: '/v1/points/award', description: 'Award points to a user', auth: true },
      { method: 'POST', path: '/v1/points/deduct', description: 'Deduct points from a user', auth: true },
      { method: 'GET', path: '/v1/points/{user_id}', description: 'Get user point balance', auth: true },
      { method: 'GET', path: '/v1/points/{user_id}/history', description: 'Get point transaction history', auth: true },
    ],
  },
  {
    category: 'Badges',
    items: [
      { method: 'POST', path: '/v1/badges/award', description: 'Award a badge to a user', auth: true },
      { method: 'GET', path: '/v1/badges', description: 'List all badges', auth: true },
      { method: 'GET', path: '/v1/badges/{user_id}', description: "Get user's earned badges", auth: true },
      { method: 'POST', path: '/v1/badges', description: 'Create a new badge', auth: true },
    ],
  },
  {
    category: 'Levels',
    items: [
      { method: 'GET', path: '/v1/levels', description: 'List all level tiers', auth: true },
      { method: 'GET', path: '/v1/levels/{user_id}', description: "Get user's current level", auth: true },
      { method: 'POST', path: '/v1/levels', description: 'Create a new level tier', auth: true },
    ],
  },
  {
    category: 'Users',
    items: [
      { method: 'GET', path: '/v1/users', description: 'List all users', auth: true },
      { method: 'GET', path: '/v1/users/{user_id}', description: 'Get user details', auth: true },
      { method: 'POST', path: '/v1/users', description: 'Create a new user', auth: true },
      { method: 'DELETE', path: '/v1/users/{user_id}', description: 'Delete a user', auth: true },
    ],
  },
  {
    category: 'Events',
    items: [
      { method: 'POST', path: '/v1/events', description: 'Track a user event', auth: true },
      { method: 'GET', path: '/v1/events/{user_id}', description: 'Get user event history', auth: true },
    ],
  },
];

const methodColors: Record<string, string> = {
  GET: 'bg-green-500/20 text-green-500',
  POST: 'bg-blue-500/20 text-blue-500',
  PUT: 'bg-amber-500/20 text-amber-500',
  DELETE: 'bg-red-500/20 text-red-500',
};

export default function ApiReference() {
  const [copiedPath, setCopiedPath] = useState<string | null>(null);
  const [selectedEndpoint, setSelectedEndpoint] = useState(endpoints[0].items[0]);

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedPath(text);
    setTimeout(() => setCopiedPath(null), 2000);
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold mb-2">API Reference</h1>
        <p className="text-muted-foreground">
          Complete REST API documentation for integrating with LevelUpOs.
        </p>
      </div>

      {/* Base URL */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Base URL</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 bg-secondary/50 rounded-lg p-3">
            <code className="text-sm flex-1">https://api.levelupos.com</code>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => copyToClipboard('https://api.levelupos.com')}
            >
              {copiedPath === 'https://api.levelupos.com' ? (
                <Check className="w-4 h-4 text-green-500" />
              ) : (
                <Copy className="w-4 h-4" />
              )}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Authentication */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Authentication</CardTitle>
          <CardDescription>All API requests require authentication via Bearer token</CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
            <code>{`Authorization: Bearer YOUR_API_KEY`}</code>
          </pre>
        </CardContent>
      </Card>

      {/* Endpoints */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Endpoint List */}
        <Card className="lg:col-span-1">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <Code className="w-5 h-5 text-primary" />
              Endpoints
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="max-h-[600px] overflow-y-auto">
              {endpoints.map((group) => (
                <div key={group.category}>
                  <div className="px-4 py-2 bg-secondary/30 text-sm font-medium text-muted-foreground sticky top-0">
                    {group.category}
                  </div>
                  {group.items.map((endpoint) => (
                    <button
                      key={endpoint.path}
                      onClick={() => setSelectedEndpoint(endpoint)}
                      className={cn(
                        "w-full flex items-center gap-2 px-4 py-3 text-left hover:bg-secondary/50 transition-colors border-b border-border/30",
                        selectedEndpoint.path === endpoint.path && "bg-secondary/50"
                      )}
                    >
                      <Badge variant="secondary" className={cn("text-[10px] font-mono", methodColors[endpoint.method])}>
                        {endpoint.method}
                      </Badge>
                      <span className="text-sm truncate flex-1 font-mono">{endpoint.path}</span>
                      <ChevronRight className="w-4 h-4 text-muted-foreground shrink-0" />
                    </button>
                  ))}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Endpoint Detail */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <div className="flex items-center gap-3">
              <Badge variant="secondary" className={cn("text-xs font-mono", methodColors[selectedEndpoint.method])}>
                {selectedEndpoint.method}
              </Badge>
              <code className="text-lg font-mono">{selectedEndpoint.path}</code>
              {selectedEndpoint.auth ? (
                <Lock className="w-4 h-4 text-amber-500" />
              ) : (
                <Unlock className="w-4 h-4 text-green-500" />
              )}
            </div>
            <CardDescription>{selectedEndpoint.description}</CardDescription>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="request">
              <TabsList>
                <TabsTrigger value="request">Request</TabsTrigger>
                <TabsTrigger value="response">Response</TabsTrigger>
              </TabsList>
              <TabsContent value="request" className="mt-4">
                <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                  <code>{`curl -X ${selectedEndpoint.method} https://api.levelupos.com${selectedEndpoint.path} \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json"${selectedEndpoint.method === 'POST' ? ` \\
  -d '{
    "user_id": "usr_123",
    "amount": 100
  }'` : ''}`}</code>
                </pre>
              </TabsContent>
              <TabsContent value="response" className="mt-4">
                <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                  <code>{`{
  "success": true,
  "data": {
    "id": "txn_abc123",
    "user_id": "usr_123",
    "created_at": "2024-03-20T14:30:00Z"
  }
}`}</code>
                </pre>
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}