'use client'

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Code, Copy, Check, Terminal, FileCode } from "lucide-react";
import { PORTAL_ROUTES } from "@/lib/constants";

const codeExamples = {
  awardBadge: {
    title: "Award a Badge",
    description: "Award a badge to a user when they complete an achievement",
    javascript: `import { LevelUpOS } from '@levelupos/sdk';

const client = new LevelUpOS({
  apiKey: 'your-api-key',
  orgId: 'your-org-id'
});

// Award a badge to a user
const response = await client.badges.award({
  userId: 'user_123',
  badgeId: 'first_purchase',
  tier: 'gold',
  metadata: {
    purchaseAmount: 99.99,
    productId: 'prod_456'
  }
});

console.log(response.badge);
// { id: 'badge_789', name: 'First Purchase', tier: 'gold', ... }`,
    python: `from levelupos import LevelUpOS

client = LevelUpOS(
    api_key="your-api-key",
    org_id="your-org-id"
)

# Award a badge to a user
response = client.badges.award(
    user_id="user_123",
    badge_id="first_purchase",
    tier="gold",
    metadata={
        "purchase_amount": 99.99,
        "product_id": "prod_456"
    }
)

print(response.badge)
# {'id': 'badge_789', 'name': 'First Purchase', 'tier': 'gold', ...}`,
    curl: `curl -X POST https://api.levelupos.com/v1/badges/award \\
  -H "Authorization: Bearer your-api-key" \\
  -H "X-Org-Id: your-org-id" \\
  -H "Content-Type: application/json" \\
  -d '{
    "userId": "user_123",
    "badgeId": "first_purchase",
    "tier": "gold",
    "metadata": {
      "purchaseAmount": 99.99,
      "productId": "prod_456"
    }
  }'`
  },
  addPoints: {
    title: "Add Points",
    description: "Credit points to a user's wallet with transaction tracking",
    javascript: `import { LevelUpOS } from '@levelupos/sdk';

const client = new LevelUpOS({
  apiKey: 'your-api-key',
  orgId: 'your-org-id'
});

// Add points to user wallet
const transaction = await client.points.credit({
  userId: 'user_123',
  amount: 500,
  reason: 'purchase_reward',
  reference: 'order_789',
  metadata: {
    orderTotal: 49.99
  }
});

console.log(transaction);
// { id: 'txn_abc', balance: 1500, amount: 500, ... }`,
    python: `from levelupos import LevelUpOS

client = LevelUpOS(
    api_key="your-api-key",
    org_id="your-org-id"
)

# Add points to user wallet
transaction = client.points.credit(
    user_id="user_123",
    amount=500,
    reason="purchase_reward",
    reference="order_789",
    metadata={
        "order_total": 49.99
    }
)

print(transaction)
# {'id': 'txn_abc', 'balance': 1500, 'amount': 500, ...}`,
    curl: `curl -X POST https://api.levelupos.com/v1/points/credit \\
  -H "Authorization: Bearer your-api-key" \\
  -H "X-Org-Id: your-org-id" \\
  -H "Content-Type: application/json" \\
  -d '{
    "userId": "user_123",
    "amount": 500,
    "reason": "purchase_reward",
    "reference": "order_789",
    "metadata": {
      "orderTotal": 49.99
    }
  }'`
  },
  createMission: {
    title: "Create Mission",
    description: "Set up a new mission with custom objectives and rewards",
    javascript: `import { LevelUpOS } from '@levelupos/sdk';

const client = new LevelUpOS({
  apiKey: 'your-api-key',
  orgId: 'your-org-id'
});

// Create a weekly mission
const mission = await client.missions.create({
  name: 'Weekly Warrior',
  type: 'weekly',
  objectives: [
    { action: 'purchase', count: 3 },
    { action: 'review', count: 2 }
  ],
  rewards: {
    points: 1000,
    badgeId: 'weekly_warrior'
  }
});

console.log(mission.id);
// 'mission_xyz'`,
    python: `from levelupos import LevelUpOS

client = LevelUpOS(
    api_key="your-api-key",
    org_id="your-org-id"
)

# Create a weekly mission
mission = client.missions.create(
    name="Weekly Warrior",
    type="weekly",
    objectives=[
        {"action": "purchase", "count": 3},
        {"action": "review", "count": 2}
    ],
    rewards={
        "points": 1000,
        "badge_id": "weekly_warrior"
    }
)

print(mission.id)
# 'mission_xyz'`,
    curl: `curl -X POST https://api.levelupos.com/v1/missions \\
  -H "Authorization: Bearer your-api-key" \\
  -H "X-Org-Id: your-org-id" \\
  -H "Content-Type: application/json" \\
  -d '{
    "name": "Weekly Warrior",
    "type": "weekly",
    "objectives": [
      { "action": "purchase", "count": 3 },
      { "action": "review", "count": 2 }
    ],
    "rewards": {
      "points": 1000,
      "badgeId": "weekly_warrior"
    }
  }'`
  }
};

type Language = 'javascript' | 'python' | 'curl';
type Example = keyof typeof codeExamples;

const languageConfig: Record<Language, { label: string; icon: React.ReactNode }> = {
  javascript: { label: 'JavaScript', icon: <FileCode className="w-4 h-4" /> },
  python: { label: 'Python', icon: <Code className="w-4 h-4" /> },
  curl: { label: 'cURL', icon: <Terminal className="w-4 h-4" /> }
};

const APIPreviewSection = () => {
  const [activeExample, setActiveExample] = useState<Example>('awardBadge');
  const [activeLanguage, setActiveLanguage] = useState<Language>('javascript');
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    const code = codeExamples[activeExample][activeLanguage];
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const currentExample = codeExamples[activeExample];

  return (
    <section className="py-24 relative overflow-hidden">
      {/* Background */}
      <div className="absolute inset-0 bg-gradient-to-b from-background via-muted/20 to-background" />

      <div className="container mx-auto px-4 relative z-10">
        {/* Header */}
        <div className="text-center mb-16">
          <span className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-cyan/10 text-cyan text-sm font-medium mb-6">
            <Code className="w-4 h-4" />
            Developer Experience
          </span>
          <h2 className="text-4xl md:text-5xl font-bold font-display mb-6">
            <span className="bg-gradient-to-r from-foreground to-foreground/70 bg-clip-text text-transparent">
              Beautiful API, Simple Integration
            </span>
          </h2>
          <p className="text-xl text-muted-foreground max-w-2xl mx-auto">
            Get started in minutes with our intuitive SDKs and comprehensive documentation
          </p>
        </div>

        <div className="max-w-5xl mx-auto">
          {/* Example Selector */}
          <div className="flex flex-wrap gap-3 mb-6 justify-center">
            {(Object.keys(codeExamples) as Example[]).map((key) => (
              <button
                key={key}
                onClick={() => setActiveExample(key)}
                className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                  activeExample === key
                    ? 'bg-cyan text-background'
                    : 'bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground'
                }`}
              >
                {codeExamples[key].title}
              </button>
            ))}
          </div>

          {/* Code Block */}
          <div className="rounded-2xl overflow-hidden border border-border/50 bg-[#0d1117] shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-3 bg-[#161b22] border-b border-border/30">
              <div className="flex items-center gap-4">
                {/* Language Tabs */}
                <div className="flex gap-1">
                  {(Object.keys(languageConfig) as Language[]).map((lang) => (
                    <button
                      key={lang}
                      onClick={() => setActiveLanguage(lang)}
                      className={`flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
                        activeLanguage === lang
                          ? 'bg-cyan/20 text-cyan'
                          : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
                      }`}
                    >
                      {languageConfig[lang].icon}
                      {languageConfig[lang].label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Copy Button */}
              <Button
                variant="ghost"
                size="sm"
                onClick={handleCopy}
                className="text-muted-foreground hover:text-foreground"
              >
                {copied ? (
                  <Check className="w-4 h-4 text-green-500" />
                ) : (
                  <Copy className="w-4 h-4" />
                )}
                <span className="ml-2">{copied ? 'Copied!' : 'Copy'}</span>
              </Button>
            </div>

            {/* Description */}
            <div className="px-6 py-3 border-b border-border/20 bg-[#161b22]/50">
              <p className="text-sm text-muted-foreground">
                {currentExample.description}
              </p>
            </div>

            {/* Code */}
            <div className="p-6 overflow-x-auto">
              <pre className="text-sm leading-relaxed">
                <code className="text-foreground/90 font-mono">
                  {currentExample[activeLanguage]}
                </code>
              </pre>
            </div>
          </div>

          {/* Footer CTA */}
          <div className="mt-8 text-center">
            <p className="text-muted-foreground mb-4">
              Explore our full API reference with interactive examples
            </p>
            <div className="flex gap-4 justify-center">
              <Button variant="heroOutline" className="gap-2" asChild>
                <a href={PORTAL_ROUTES.DOCS}>
                  <FileCode className="w-4 h-4" />
                  View Full Docs
                </a>
              </Button>
              <Button variant="ghost" className="text-cyan hover:text-cyan/80" asChild>
                <a href={`${PORTAL_ROUTES.DASHBOARD}/playground`}>
                  Try in Playground →
                </a>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default APIPreviewSection;
