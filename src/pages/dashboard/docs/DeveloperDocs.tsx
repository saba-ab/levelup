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
          <CardDescription>Install our SDK for your preferred language</CardDescription>
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
                  <Badge variant="secondary">v{sdk.version}</Badge>
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
              <TabsTrigger value="javascript">JavaScript</TabsTrigger>
              <TabsTrigger value="python">Python</TabsTrigger>
              <TabsTrigger value="curl">cURL</TabsTrigger>
            </TabsList>
            <TabsContent value="javascript" className="mt-4">
              <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                <code>{`import { LevelUpOs } from '@levelupos/js-sdk';

// Initialize the client
const client = new LevelUpOs({
  apiKey: 'YOUR_API_KEY',
  environment: 'production'
});

// Award points to a user
const result = await client.points.award({
  userId: 'usr_123',
  points: 100,
  reason: 'Completed onboarding'
});

// Track a custom event
await client.events.track({
  userId: 'usr_123',
  event: 'purchase_completed',
  properties: {
    orderId: 'ord_456',
    amount: 99.99
  }
});

console.log('Points awarded:', result.data);`}</code>
              </pre>
            </TabsContent>
            <TabsContent value="python" className="mt-4">
              <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                <code>{`from levelupos import LevelUpOs

# Initialize the client
client = LevelUpOs(
    api_key="YOUR_API_KEY",
    environment="production"
)

# Award points to a user
result = client.points.award(
    user_id="usr_123",
    points=100,
    reason="Completed onboarding"
)

# Track a custom event
client.events.track(
    user_id="usr_123",
    event="purchase_completed",
    properties={
        "order_id": "ord_456",
        "amount": 99.99
    }
)

print(f"Points awarded: {result.data}")`}</code>
              </pre>
            </TabsContent>
            <TabsContent value="curl" className="mt-4">
              <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
                <code>{`# Award points to a user
curl -X POST https://api.levelupos.com/v1/points/award \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "user_id": "usr_123",
    "points": 100,
    "reason": "Completed onboarding"
  }'

# Track a custom event
curl -X POST https://api.levelupos.com/v1/events \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "user_id": "usr_123",
    "event": "purchase_completed",
    "properties": {
      "order_id": "ord_456",
      "amount": 99.99
    }
  }'`}</code>
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