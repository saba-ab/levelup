import { BookOpen, Terminal } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { useEnvironment } from '@/contexts/EnvironmentContext';
import { CodeBlock } from './CodeBlock';

const DEFAULT_BASE_URL = 'https://api.levelupos.ge';

function tsQuickstart(baseUrl: string) {
  const base = baseUrl === DEFAULT_BASE_URL ? `  // baseUrl: "${DEFAULT_BASE_URL}", (default; the SDK appends /api/v1)` : `  baseUrl: "${baseUrl}",`;
  return `import { LevelUp } from "@levelup/sdk";

const levelup = new LevelUp({
  apiKey: process.env.LEVELUP_API_KEY!, // lvl_live_…
${base}
});

// 1. Report an activity. The rules engine decides what it earns.
const res = await levelup.activities.send({
  event_id: "order-1042",          // your unique id; re-sending it is deduplicated
  event_type: "purchase",
  player_external_id: "user-7",
  properties: { amount: 59.9, currency: "GEL" },
  occurred_at: new Date(),
});
console.log(res.status, res.duplicate);

// 2. Credit points directly (always idempotent).
const player = await levelup.players.getByExternalId("user-7");
const entry = await levelup.wallets.credit(
  player.id,
  { amount: 100, kind: "bonus", description: "Welcome bonus" },
  { idempotencyKey: \`welcome-\${player.id}\` },
);
console.log(entry.balance_after);`;
}

function pyQuickstart(baseUrl: string) {
  const base = baseUrl === DEFAULT_BASE_URL ? `    # base_url="${DEFAULT_BASE_URL}",  (default; the SDK appends /api/v1)` : `    base_url="${baseUrl}",`;
  return `import os
import levelup

client = levelup.Client(
    api_key=os.environ["LEVELUP_API_KEY"],  # lvl_live_…
${base}
)

# 1. Report an activity. The rules engine decides what it earns.
res = client.activities.send(
    event_id="order-1042",                 # your unique id; re-sending it is deduplicated
    event_type="purchase",
    player_external_id="user-7",
    properties={"amount": 59.9, "currency": "GEL"},
)
print(res["status"], res.get("duplicate"))

# 2. Credit points directly (always idempotent).
player = client.players.get_by_external_id("user-7")
entry = client.wallets.credit(
    player["id"], 100, kind="bonus", description="Welcome bonus",
    idempotency_key=f"welcome-{player['id']}",
)
print(entry["balance_after"])`;
}

const TS_WEBHOOK = `import { verifyWebhook, WebhookVerificationError } from "@levelup/sdk";

// rawBody: the unparsed request body (Buffer or string)
const event = verifyWebhook(rawBody, req.get("LevelUp-Signature"), process.env.LEVELUP_WEBHOOK_SECRET!);
// event: { event, event_id, occurred_at, tenant_id, data }`;

const PY_WEBHOOK = `import levelup

# raw_body: the unparsed request bytes (e.g. Flask request.get_data())
event = levelup.verify_webhook(raw_body, headers.get(levelup.SIGNATURE_HEADER), os.environ["LEVELUP_WEBHOOK_SECRET"])
# event: {"event", "event_id", "occurred_at", "tenant_id", "data"}`;

function curlExample(baseUrl: string) {
  return `curl -X POST ${baseUrl}/api/v1/activities \\
  -H "Authorization: Bearer lvl_live_…" \\
  -H "Content-Type: application/json" \\
  -d '{
    "event_id": "order-1042",
    "event_type": "purchase",
    "player_external_id": "user-7",
    "properties": { "amount": 59.9, "currency": "GEL" }
  }'

# 202 {"status":"pending", ...} for a new activity;
# 200 with "duplicate": true when the event_id was already ingested.`;
}

/** Integrations → SDKs: install and quick-start snippets for the official SDKs. */
export function SdkPanel() {
  const { activeEnvironment } = useEnvironment();
  const baseUrl = (activeEnvironment?.url || DEFAULT_BASE_URL).replace(/\/+$/, '');

  return (
    <div className="space-y-6">
      <Alert>
        <Terminal className="h-4 w-4" />
        <AlertDescription>
          The SDKs are server-side. API keys are secrets: never ship them to a browser or a mobile app. Snippets use
          the API of the environment you are connected to (<code className="text-xs">{baseUrl}</code>).
        </AlertDescription>
      </Alert>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">TypeScript / Node.js</CardTitle>
            <CardDescription>
              <code>@levelup/sdk</code>: zero dependencies, Node.js 18+, ESM and CommonJS, typed requests and responses.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <CodeBlock title="Install" code="npm install @levelup/sdk" />
            <CodeBlock title="Quick start" code={tsQuickstart(baseUrl)} />
            <CodeBlock title="Verify a webhook" code={TS_WEBHOOK} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Python</CardTitle>
            <CardDescription>
              <code>levelup</code>: standard library only, Python 3.9+, responses typed with <code>TypedDict</code>s.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <CodeBlock title="Install" code="pip install levelup" />
            <CodeBlock title="Quick start" code={pyQuickstart(baseUrl)} />
            <CodeBlock title="Verify a webhook" code={PY_WEBHOOK} />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <BookOpen className="h-4 w-4" /> Raw HTTP
          </CardTitle>
          <CardDescription>
            Any language works: send the API key as a bearer token. Errors are <code>application/problem+json</code>{' '}
            with a stable <code>code</code> to branch on.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <CodeBlock title="curl: send an activity" code={curlExample(baseUrl)} />
        </CardContent>
      </Card>
    </div>
  );
}
