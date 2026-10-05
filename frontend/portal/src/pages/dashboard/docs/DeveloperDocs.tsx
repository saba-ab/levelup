import React, { useState } from 'react';
import { Terminal, Package, Github, Copy, Check, ExternalLink, Code2, Boxes, Webhook, Shield } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

const sdks = [
  {
    name: 'JavaScript / TypeScript',
    package: '@levelupos/js-sdk',
    version: '2.4.1',
    install: 'npm install @levelupos/js-sdk',
    icon: '🟨',
  },
  {
    name: 'Python',
    package: 'levelupos-python',
    version: '1.8.0',
    install: 'pip install levelupos-python',
    icon: '🐍',
  },
  {
    name: 'Ruby',
    package: 'levelupos-ruby',
    version: '1.3.2',
    install: 'gem install levelupos-ruby',
    icon: '💎',
  },
  {
    name: 'Go',
    package: 'github.com/levelupos/go-sdk',
    version: '0.9.1',
    install: 'go get github.com/levelupos/go-sdk',
    icon: '🔷',
  },
];

const integrationPatterns = [
  {
    title: 'Event-Driven Integration',
    description: 'Track user events and trigger gamification rules automatically',
    icon: Boxes,
  },
  {
    title: 'Webhook Notifications',
    description: 'Receive real-time notifications when gamification events occur',
    icon: Webhook,
  },
  {
    title: 'Embedded Widgets',
    description: 'Add pre-built UI components to display progress and achievements',
    icon: Code2,
  },
  {
    title: 'Secure Server-to-Server',
    description: 'Backend integration with secure API authentication',
    icon: Shield,
  },
];

export default function DeveloperDocs() {
  const [copiedText, setCopiedText] = useState<string | null>(null);

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedText(text);
    setTimeout(() => setCopiedText(null), 2000);
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold mb-2">Developer Documentation</h1>
        <p className="text-muted-foreground">
          SDKs, integration patterns, and code examples for developers.
        </p>
      </div>

      {/* SDKs */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Package className="w-5 h-5 text-primary" />
            Official SDKs
          </CardTitle>
          <CardDescription>Official SDKs are coming soon. Until then, call the REST API directly (examples below).</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {sdks.map((sdk) => (
              <div
                key={sdk.package}
                className="flex items-center justify-between p-4 rounded-lg bg-secondary/30 hover:bg-secondary/50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <span className="text-2xl">{sdk.icon}</span>
                  <div>
                    <h3 className="font-medium">{sdk.name}</h3>
                    <p className="text-sm text-muted-foreground font-mono">{sdk.package}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="secondary">Coming soon</Badge>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => copyToClipboard(sdk.install)}
                  >
                    {copiedText === sdk.install ? (
                      <Check className="w-4 h-4 text-green-500" />
                    ) : (
                      <Copy className="w-4 h-4" />
                    )}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Code Examples */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Terminal className="w-5 h-5 text-primary" />
            Quick Start Examples
          </CardTitle>
          <CardDescription>Get started with these code snippets</CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="javascript">
            <TabsList>
              <TabsTrigger value="javascript">JavaScript (fetch)</TabsTrigger>
              <TabsTrigger value="python">Python (requests)</TabsTrigger>
              <TabsTrigger value="curl">cURL</TabsTrigger>
            </TabsList>
            <TabsContent value="javascript" className="mt-4">
              <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                <code>{`// Report an activity; live rules award points, XP, badges… asynchronously.
const res = await fetch('https://api.levelupos.ge/api/v1/activities', {
  method: 'POST',
  headers: {
    'Authorization': \`Bearer \${accessToken}\`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    event_id: 'order-456',            // idempotency key: resending is safe
    event_type: 'purchase_completed',
    player_external_id: 'usr_123',
    properties: { order_id: 'ord_456', amount: 99.99 },
  }),
});

const { activity_id, status, duplicate } = await res.json(); // 202 { status: "pending" }`}</code>
              </pre>
            </TabsContent>
            <TabsContent value="python" className="mt-4">
              <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                <code>{`import requests

# Report an activity; live rules react to it asynchronously.
res = requests.post(
    "https://api.levelupos.ge/api/v1/activities",
    headers={"Authorization": f"Bearer {access_token}"},
    json={
        "event_id": "order-456",          # idempotency key: resending is safe
        "event_type": "purchase_completed",
        "player_external_id": "usr_123",
        "properties": {"order_id": "ord_456", "amount": 99.99},
    },
)
print(res.status_code, res.json())  # 202 {"activity_id": ..., "status": "pending"}`}</code>
              </pre>
            </TabsContent>
            <TabsContent value="curl" className="mt-4">
              <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                <code>{`# Sign in (returns access_token and refresh_token)
curl -X POST https://api.levelupos.ge/api/v1/auth/login \\
  -H "Content-Type: application/json" \\
  -d '{ "email": "you@company.com", "password": "..." }'

# Report an activity
curl -X POST https://api.levelupos.ge/api/v1/activities \\
  -H "Authorization: Bearer ACCESS_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "event_id": "order-456",
    "event_type": "purchase_completed",
    "player_external_id": "usr_123",
    "properties": { "order_id": "ord_456", "amount": 99.99 }
  }'

# Credit points directly (Idempotency-Key makes retries safe)
curl -X POST https://api.levelupos.ge/api/v1/players/PLAYER_ID/wallet/credit \\
  -H "Authorization: Bearer ACCESS_TOKEN" \\
  -H "Idempotency-Key: 7d1c6f0e-onboarding" \\
  -H "Content-Type: application/json" \\
  -d '{ "amount": 100, "kind": "bonus", "description": "Completed onboarding" }'`}</code>
              </pre>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {/* Integration Patterns */}
      <Card>
        <CardHeader>
          <CardTitle>Integration Patterns</CardTitle>
          <CardDescription>Choose the best integration approach for your use case</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {integrationPatterns.map((pattern) => (
              <div
                key={pattern.title}
                className="flex items-start gap-4 p-4 rounded-lg bg-secondary/30 hover:bg-secondary/50 transition-colors cursor-pointer"
              >
                <div className="p-2 rounded-md bg-primary/10">
                  <pattern.icon className="w-5 h-5 text-primary" />
                </div>
                <div>
                  <h3 className="font-medium">{pattern.title}</h3>
                  <p className="text-sm text-muted-foreground">{pattern.description}</p>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Resources */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card className="hover:border-primary/50 transition-colors cursor-pointer">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="p-3 rounded-lg bg-secondary/50">
                <Github className="w-6 h-6" />
              </div>
              <div className="flex-1">
                <h3 className="font-medium">GitHub Examples</h3>
                <p className="text-sm text-muted-foreground">Browse complete example projects</p>
              </div>
              <ExternalLink className="w-5 h-5 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>
        <Card className="hover:border-primary/50 transition-colors cursor-pointer">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="p-3 rounded-lg bg-secondary/50">
                <Terminal className="w-6 h-6" />
              </div>
              <div className="flex-1">
                <h3 className="font-medium">API Playground</h3>
                <p className="text-sm text-muted-foreground">Test API calls interactively</p>
              </div>
              <ExternalLink className="w-5 h-5 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}