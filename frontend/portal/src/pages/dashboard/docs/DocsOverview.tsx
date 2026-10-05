import React from 'react';
import { Link } from 'react-router-dom';
import { Book, Code, FileText, Rocket, Terminal, Webhook, Key, Zap } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

const quickLinks = [
  {
    title: 'API Reference',
    description: 'Complete REST API documentation with endpoints, parameters, and examples',
    icon: Code,
    path: '/docs/api',
    color: 'text-blue-500',
    bgColor: 'bg-blue-500/10',
  },
  {
    title: 'User Guides',
    description: 'Step-by-step tutorials for setting up and managing gamification features',
    icon: Book,
    path: '/docs/guides',
    color: 'text-green-500',
    bgColor: 'bg-green-500/10',
  },
  {
    title: 'Developer Docs',
    description: 'SDKs, integration patterns, and code examples for developers',
    icon: Terminal,
    path: '/docs/developer',
    color: 'text-purple-500',
    bgColor: 'bg-purple-500/10',
  },
];

const gettingStarted = [
  { title: 'Quick Start', description: 'Get up and running in 5 minutes', icon: Rocket },
  { title: 'Authentication', description: 'Learn how to authenticate API requests', icon: Key },
  { title: 'Webhooks', description: 'Set up real-time event notifications', icon: Webhook },
  { title: 'Rate Limits', description: 'Understand API usage limits and best practices', icon: Zap },
];

export default function DocsOverview() {
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold mb-2">Documentation</h1>
        <p className="text-muted-foreground">
          Everything you need to integrate and use the LevelUpOs gamification platform.
        </p>
      </div>

      {/* Quick Links */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {quickLinks.map((link) => (
          <Link key={link.path} to={link.path}>
            <Card className="h-full hover:border-primary/50 hover:shadow-lg hover:shadow-primary/5 transition-all cursor-pointer group">
              <CardHeader>
                <div className={`w-12 h-12 rounded-lg ${link.bgColor} flex items-center justify-center mb-4 group-hover:scale-110 transition-transform`}>
                  <link.icon className={`w-6 h-6 ${link.color}`} />
                </div>
                <CardTitle className="group-hover:text-primary transition-colors">{link.title}</CardTitle>
                <CardDescription>{link.description}</CardDescription>
              </CardHeader>
            </Card>
          </Link>
        ))}
      </div>

      {/* Getting Started */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FileText className="w-5 h-5 text-primary" />
            Getting Started
          </CardTitle>
          <CardDescription>Essential resources to begin your integration</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {gettingStarted.map((item) => (
              <div
                key={item.title}
                className="flex items-start gap-4 p-4 rounded-lg bg-secondary/30 hover:bg-secondary/50 transition-colors cursor-pointer"
              >
                <div className="p-2 rounded-md bg-primary/10">
                  <item.icon className="w-5 h-5 text-primary" />
                </div>
                <div>
                  <h3 className="font-medium">{item.title}</h3>
                  <p className="text-sm text-muted-foreground">{item.description}</p>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Code Example */}
      <Card>
        <CardHeader>
          <CardTitle>Quick Example</CardTitle>
          <CardDescription>Report an activity; your live rules decide what the player earns</CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="bg-secondary/50 rounded-lg p-4 overflow-x-auto text-sm">
            <code className="text-foreground">{`curl -X POST https://api.levelupos.ge/api/v1/activities \\
  -H "Authorization: Bearer $LEVELUP_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "event_id": "order-1042",
    "event_type": "purchase_completed",
    "player_external_id": "usr_123",
    "properties": { "amount": 150 }
  }'`}</code>
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}