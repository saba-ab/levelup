import React, { useState } from 'react';
import { Code, ChevronRight, Copy, Check, Lock, Unlock } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';
import { useEnvironment } from '@/contexts/EnvironmentContext';

interface Endpoint {
  method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  path: string;
  description: string;
  auth: boolean;
  body?: string;
  response?: string;
}

const LIST_RESPONSE = `{
  "data": [ { "id": "01a10d23-0055-7290-9a84-e33fee1fcc63", ... } ],
  "next_cursor": "MTc5MTIy..."
}`;

/** A selection of the Go API (/api/v1). The full contract is the OpenAPI document (swagger.json). */
const endpoints: { category: string; items: Endpoint[] }[] = [
  {
    category: 'Auth',
    items: [
      {
        method: 'POST',
        path: '/api/v1/auth/login',
        description: 'Sign in; returns an access token (Bearer) and a single-use refresh token',
        auth: false,
        body: `{ "email": "you@company.com", "password": "••••••••" }`,
        response: `{
  "user": { "id": "...", "email": "you@company.com", "roles": [...] },
  "tenant": { "id": "...", "name": "Acme" },
  "access_token": "eyJ...",
  "refresh_token": "...",
  "token_type": "Bearer",
  "expires_in": 900
}`,
      },
      { method: 'POST', path: '/api/v1/auth/refresh', description: 'Exchange a refresh token for a new session', auth: false, body: `{ "refresh_token": "..." }` },
      { method: 'GET', path: '/api/v1/auth/me', description: 'Current user and tenant', auth: true },
    ],
  },
  {
    category: 'Activities',
    items: [
      {
        method: 'POST',
        path: '/api/v1/activities',
        description: 'Report an activity; rules evaluate it asynchronously (idempotent on event_id)',
        auth: true,
        body: `{
  "event_id": "order-1042",
  "event_type": "purchase_completed",
  "player_external_id": "user-123",
  "properties": { "amount": 150 }
}`,
        response: `{ "activity_id": "01a10d23-...", "status": "pending", "duplicate": false }`,
      },
      { method: 'POST', path: '/api/v1/activities/batch', description: 'Report up to 100 activities at once', auth: true, body: `{ "items": [ { "event_id": "...", "event_type": "...", "player_external_id": "..." } ] }` },
      { method: 'GET', path: '/api/v1/activities', description: 'List activities (?status=pending|decided|rejected)', auth: true, response: LIST_RESPONSE },
    ],
  },
  {
    category: 'Players',
    items: [
      { method: 'GET', path: '/api/v1/players', description: 'List players (cursor paginated)', auth: true, response: LIST_RESPONSE },
      { method: 'POST', path: '/api/v1/players', description: 'Create a player', auth: true, body: `{ "external_id": "user-123", "display_name": "Ada" }` },
      { method: 'GET', path: '/api/v1/players/by-external-id/{external_id}', description: 'Find a player by your id', auth: true },
      { method: 'GET', path: '/api/v1/players/{id}/wallet', description: "Player's points balance", auth: true },
      { method: 'POST', path: '/api/v1/players/{id}/wallet/credit', description: 'Credit points (send an Idempotency-Key header)', auth: true, body: `{ "amount": 100, "kind": "bonus", "description": "Welcome bonus" }` },
      { method: 'GET', path: '/api/v1/players/{id}/badges', description: "Player's earned badges", auth: true },
      { method: 'GET', path: '/api/v1/players/{id}/progress', description: "Player's level and XP", auth: true },
    ],
  },
  {
    category: 'Rules',
    items: [
      { method: 'GET', path: '/api/v1/rules', description: 'List rules (?status=&trigger_event=)', auth: true, response: LIST_RESPONSE },
      {
        method: 'POST',
        path: '/api/v1/rules',
        description: 'Create a rule (draft version 1)',
        auth: true,
        body: `{
  "name": "Big purchase bonus",
  "trigger_event": "purchase_completed",
  "conditions": [ { "source": "trigger", "field": "amount", "operator": "gte", "value": 100 } ],
  "actions": [ { "type": "credit_points", "amount": 50 } ],
  "limits": { "max_per_player_per_day": 1 }
}`,
      },
      { method: 'POST', path: '/api/v1/rules/{id}/publish', description: 'Make a version live', auth: true, body: `{ "version": 1 }` },
      {
        method: 'POST',
        path: '/api/v1/rules/simulate',
        description: 'Evaluate a hypothetical activity against live rules (no writes)',
        auth: true,
        body: `{ "event_type": "purchase_completed", "player_external_id": "user-123", "properties": { "amount": 150 } }`,
      },
      { method: 'GET', path: '/api/v1/rules/decisions', description: 'Decisions (?activity_id=&player_id=)', auth: true, response: LIST_RESPONSE },
    ],
  },
  {
    category: 'Catalogue',
    items: [
      { method: 'GET', path: '/api/v1/events', description: 'Event types (triggers), global and tenant', auth: true, response: LIST_RESPONSE },
      { method: 'GET', path: '/api/v1/badges', description: 'List badges', auth: true, response: LIST_RESPONSE },
      { method: 'POST', path: '/api/v1/badges/{id}/award', description: 'Award a badge (send an Idempotency-Key header)', auth: true, body: `{ "player_id": "..." }` },
      { method: 'GET', path: '/api/v1/levels', description: 'List levels', auth: true, response: LIST_RESPONSE },
      { method: 'GET', path: '/api/v1/leaderboards/{id}/entries', description: 'Leaderboard ranking for the current period', auth: true, response: LIST_RESPONSE },
    ],
  },
];

const methodColors: Record<string, string> = {
  GET: 'bg-green-500/20 text-green-500',
  POST: 'bg-blue-500/20 text-blue-500',
  PUT: 'bg-amber-500/20 text-amber-500',
  PATCH: 'bg-amber-500/20 text-amber-500',
  DELETE: 'bg-red-500/20 text-red-500',
};

export default function ApiReference() {
  const { baseUrl } = useEnvironment();
  const [copiedPath, setCopiedPath] = useState<string | null>(null);
  const [selectedEndpoint, setSelectedEndpoint] = useState<Endpoint>(endpoints[0].items[0]);

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
          Key REST endpoints for integrating with LevelUpOs. The complete contract is the API's OpenAPI document. Lists are cursor paginated (?limit=&cursor=) and errors are application/problem+json.
        </p>
      </div>

      {/* Base URL */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Base URL</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 bg-secondary/50 rounded-lg p-3">
            <code className="text-sm flex-1">{baseUrl}</code>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => copyToClipboard(baseUrl)}
            >
              {copiedPath === baseUrl ? (
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
          <CardDescription>
            Backends use an API key from Settings → API Keys. People signed in to the portal use the access token
            from POST /api/v1/auth/login (refreshed with /api/v1/auth/refresh).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
            <code>{`# Server-to-server (recommended for integrations)
Authorization: Bearer lvl_live_xxxxxxxxxx_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
# or
X-API-Key: lvl_live_xxxxxxxxxx_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`}</code>
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
                      key={`${endpoint.method} ${endpoint.path}`}
                      onClick={() => setSelectedEndpoint(endpoint)}
                      className={cn(
                        "w-full flex items-center gap-2 px-4 py-3 text-left hover:bg-secondary/50 transition-colors border-b border-border/30",
                        selectedEndpoint === endpoint && "bg-secondary/50"
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
                  <code>{`curl -X ${selectedEndpoint.method} ${baseUrl}${selectedEndpoint.path}${selectedEndpoint.auth ? ` \\
  -H "Authorization: Bearer $LEVELUP_API_KEY"` : ''} \\
  -H "Content-Type: application/json"${selectedEndpoint.body ? ` \\
  -d '${selectedEndpoint.body}'` : ''}`}</code>
                </pre>
              </TabsContent>
              <TabsContent value="response" className="mt-4">
                <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                  <code>{selectedEndpoint.response ?? '{ ... }  (see the OpenAPI document for this response)'}</code>
                </pre>
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}