import { ShieldCheck } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { CodeBlock } from './CodeBlock';

const HEADER = `LevelUp-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(secret, t + "." + raw body)>
LevelUp-Event:     <event name, e.g. badges.awarded>
LevelUp-Delivery:  <delivery id, stable across retries: dedupe on it>`;

const TS_SNIPPET = `import express from "express";
import { verifyWebhook, WebhookVerificationError } from "@levelup/sdk";

app.post("/webhooks/levelup", express.raw({ type: "application/json" }), (req, res) => {
  try {
    const event = verifyWebhook(
      req.body,                               // raw Buffer, not parsed JSON
      req.get("LevelUp-Signature"),
      process.env.LEVELUP_WEBHOOK_SECRET!,    // the whole whsec_… value
    );
    // event: { event, event_id, occurred_at, tenant_id, data }
    res.sendStatus(204);
  } catch (err) {
    if (err instanceof WebhookVerificationError) return res.status(400).send(err.code);
    throw err;
  }
});`;

const PY_SNIPPET = `import os
import levelup
from flask import Flask, request, abort

app = Flask(__name__)

@app.post("/webhooks/levelup")
def levelup_webhook():
    try:
        event = levelup.verify_webhook(
            request.get_data(),                           # raw bytes, NOT request.json
            request.headers.get(levelup.SIGNATURE_HEADER),
            os.environ["LEVELUP_WEBHOOK_SECRET"],         # the whole whsec_… value
        )
    except levelup.WebhookVerificationError as err:
        abort(400, err.code)
    return "", 204`;

const MANUAL_SNIPPET = `import crypto from "node:crypto";

function verify(rawBody: string, header: string, secret: string, toleranceSec = 300) {
  const parts = Object.fromEntries(header.split(",").map(p => p.split("=", 2)));
  const t = Number(parts.t);
  if (!t || Math.abs(Date.now() / 1000 - t) > toleranceSec) throw new Error("stale timestamp");
  // Key the HMAC with the whole secret string, including the whsec_ prefix.
  const expected = crypto.createHmac("sha256", secret).update(\`\${t}.\${rawBody}\`).digest("hex");
  const ok = header.split(",").some(p => p.startsWith("v1=") &&
    p.length === 3 + expected.length &&
    crypto.timingSafeEqual(Buffer.from(p.slice(3)), Buffer.from(expected)));
  if (!ok) throw new Error("bad signature");
  return JSON.parse(rawBody);
}`;

/** How receivers verify the LevelUp-Signature header. */
export function WebhookSignatureDocs() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" /> Verifying signatures
        </CardTitle>
        <CardDescription>
          Every delivery is signed with the endpoint's secret. The HMAC key is the whole <code>whsec_…</code> string.
          Verify against the raw request body and reject timestamps older than five minutes. Use the SDK helpers{' '}
          <code>verifyWebhook</code> (TypeScript, <code>@levelup/sdk</code>) or <code>verify_webhook</code> (Python,{' '}
          <code>levelup</code>).
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <CodeBlock title="Headers" code={HEADER} />
        <Tabs defaultValue="ts">
          <TabsList>
            <TabsTrigger value="ts">TypeScript SDK</TabsTrigger>
            <TabsTrigger value="py">Python SDK</TabsTrigger>
            <TabsTrigger value="manual">Without the SDK</TabsTrigger>
          </TabsList>
          <TabsContent value="ts"><CodeBlock title="verifyWebhook" code={TS_SNIPPET} /></TabsContent>
          <TabsContent value="py"><CodeBlock title="verify_webhook" code={PY_SNIPPET} /></TabsContent>
          <TabsContent value="manual"><CodeBlock title="Node.js" code={MANUAL_SNIPPET} /></TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
