'use client'

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Code, Copy, Check, Terminal, FileCode } from "lucide-react";
import { API_URL, PORTAL_ROUTES } from "@/lib/constants";

// Real requests against the LevelUp API (Go, /api/v1). There is no SDK yet:
// these are plain HTTP calls. Shapes match backend/api/docs/swagger.json.
const API = API_URL;

const codeExamples = {
  sendActivity: {
    title: "Send an Activity",
    description: "Report what a player did. Rules evaluate it asynchronously and award points, XP, badges and more. Safe to retry: event_id is deduplicated.",
    javascript: `// Your backend reports an activity; LevelUp's rules decide the rewards.
const res = await fetch('${API}/api/v1/activities', {
  method: 'POST',
  headers: {
    'Authorization': \`Bearer \${process.env.LEVELUP_API_KEY}\`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    event_id: 'order_789',            // your id: retries never double-award
    event_type: 'purchase_completed',
    player_external_id: 'user_123',   // the player's id in your system
    properties: { amount: 99.99, product_id: 'prod_456' },
  }),
});

console.log(res.status, await res.json());
// 202 { activity_id: '01a1...', status: 'pending', duplicate: false }`,
    python: `import os, requests

# Your backend reports an activity; LevelUp's rules decide the rewards.
res = requests.post(
    "${API}/api/v1/activities",
    headers={"Authorization": f"Bearer {os.environ['LEVELUP_API_KEY']}"},
    json={
        "event_id": "order_789",            # your id: retries never double-award
        "event_type": "purchase_completed",
        "player_external_id": "user_123",   # the player's id in your system
        "properties": {"amount": 99.99, "product_id": "prod_456"},
    },
)

print(res.status_code, res.json())
# 202 {'activity_id': '01a1...', 'status': 'pending', 'duplicate': False}`,
    curl: `curl -X POST ${API}/api/v1/activities \\
  -H "Authorization: Bearer $LEVELUP_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "event_id": "order_789",
    "event_type": "purchase_completed",
    "player_external_id": "user_123",
    "properties": { "amount": 99.99, "product_id": "prod_456" }
  }'

# 202 {"activity_id":"01a1...","status":"pending","duplicate":false}`
  },
  creditPoints: {
    title: "Credit Points",
    description: "Add points to a player's wallet. Every movement is an immutable ledger entry; the Idempotency-Key makes retries safe.",
    javascript: `const playerId = '01a10cdf-5e1c-70a0-a223-4589108b5a25';

const res = await fetch(\`${API}/api/v1/players/\${playerId}/wallet/credit\`, {
  method: 'POST',
  headers: {
    'Authorization': \`Bearer \${process.env.LEVELUP_API_KEY}\`,
    'Content-Type': 'application/json',
    'Idempotency-Key': 'order_789-points',   // required for money movements
  },
  body: JSON.stringify({ amount: 500, kind: 'earn', description: 'Purchase reward' }),
});

console.log(await res.json());
// { id: '01a1...', kind: 'earn', direction: 'credit', amount: 500,
//   balance_before: 1000, balance_after: 1500, ... }`,
    python: `import os, requests

player_id = "01a10cdf-5e1c-70a0-a223-4589108b5a25"

res = requests.post(
    f"${API}/api/v1/players/{player_id}/wallet/credit",
    headers={
        "Authorization": f"Bearer {os.environ['LEVELUP_API_KEY']}",
        "Idempotency-Key": "order_789-points",   # required for money movements
    },
    json={"amount": 500, "kind": "earn", "description": "Purchase reward"},
)

print(res.json())
# {'id': '01a1...', 'kind': 'earn', 'direction': 'credit', 'amount': 500,
#  'balance_before': 1000, 'balance_after': 1500, ...}`,
    curl: `curl -X POST ${API}/api/v1/players/01a10cdf-5e1c-70a0-a223-4589108b5a25/wallet/credit \\
  -H "Authorization: Bearer $LEVELUP_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: order_789-points" \\
  -d '{ "amount": 500, "kind": "earn", "description": "Purchase reward" }'

# Debiting more than the balance returns 422 with
# {"code":"insufficient_balance", "detail":"requested 600, available 500: ..."}`
  },
  awardBadge: {
    title: "Award a Badge",
    description: "Award a badge directly. Already earned? You get a 409 with code badge_already_earned, never a duplicate.",
    javascript: `const badgeId = '01a10ce1-26ab-761e-8dcc-6266f4f0cc94';   // "First Purchase"

const res = await fetch(\`${API}/api/v1/badges/\${badgeId}/award\`, {
  method: 'POST',
  headers: {
    'Authorization': \`Bearer \${process.env.LEVELUP_API_KEY}\`,
    'Content-Type': 'application/json',
    'Idempotency-Key': 'order_789-badge',
  },
  body: JSON.stringify({ player_id: '01a10cdf-5e1c-70a0-a223-4589108b5a25' }),
});

console.log(res.status, await res.json());
// 201 { award_id: '01a1...', status: 'applied',
//       player_badge: { badge_id: '01a10ce1...', earned_count: 1, ... } }`,
    python: `import os, requests

badge_id = "01a10ce1-26ab-761e-8dcc-6266f4f0cc94"   # "First Purchase"

res = requests.post(
    f"${API}/api/v1/badges/{badge_id}/award",
    headers={
        "Authorization": f"Bearer {os.environ['LEVELUP_API_KEY']}",
        "Idempotency-Key": "order_789-badge",
    },
    json={"player_id": "01a10cdf-5e1c-70a0-a223-4589108b5a25"},
)

print(res.status_code, res.json())
# 201 {'award_id': '01a1...', 'status': 'applied',
#      'player_badge': {'badge_id': '01a10ce1...', 'earned_count': 1, ...}}`,
    curl: `curl -X POST ${API}/api/v1/badges/01a10ce1-26ab-761e-8dcc-6266f4f0cc94/award \\
  -H "Authorization: Bearer $LEVELUP_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: order_789-badge" \\
  -d '{ "player_id": "01a10cdf-5e1c-70a0-a223-4589108b5a25" }'

# 201 {"award_id":"01a1...","status":"applied","player_badge":{...}}`
  },
};

type Language = 'javascript' | 'python' | 'curl';
type Example = keyof typeof codeExamples;

const languageConfig: Record<Language, { label: string; icon: React.ReactNode }> = {
  javascript: { label: 'JavaScript', icon: <FileCode className="w-4 h-4" /> },
  python: { label: 'Python', icon: <Code className="w-4 h-4" /> },
  curl: { label: 'cURL', icon: <Terminal className="w-4 h-4" /> }
};

const APIPreviewSection = () => {
  const [activeExample, setActiveExample] = useState<Example>('sendActivity');
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
            Plain REST and JSON: get started in minutes from any language
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
              Explore the full API reference, then preview rules against your data
            </p>
            <div className="flex gap-4 justify-center">
              <Button variant="heroOutline" className="gap-2" asChild>
                <a href={PORTAL_ROUTES.DOCS}>
                  <FileCode className="w-4 h-4" />
                  View Full Docs
                </a>
              </Button>
              <Button variant="ghost" className="text-cyan hover:text-cyan/80" asChild>
                <a href={PORTAL_ROUTES.SIMULATOR}>
                  Try the Rule Simulator →
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
