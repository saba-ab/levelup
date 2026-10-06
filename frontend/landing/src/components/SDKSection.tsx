'use client'

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Check, Copy, Package, Terminal } from "lucide-react";
import { SDK_PACKAGES } from "@/lib/constants";

type Sdk = 'typescript' | 'python';

const sdks: Record<Sdk, { label: string; install: string; code: string; notes: string[] }> = {
  typescript: {
    label: 'TypeScript',
    install: `npm install ${SDK_PACKAGES.TYPESCRIPT}`,
    code: `import { LevelUp } from '${SDK_PACKAGES.TYPESCRIPT}';

const levelup = new LevelUp({ apiKey: process.env.LEVELUP_API_KEY });

// Report what a player did; your rules decide the rewards.
const result = await levelup.activities.send({
  event_id: 'order_789',            // your id: retries never double-award
  event_type: 'purchase_completed',
  player_external_id: 'user_123',   // the player's id in your system
  properties: { amount: 99.99, product_id: 'prod_456' },
});

console.log(result.status); // 'pending': rules run asynchronously`,
    notes: ['Node.js 18+, ESM and CommonJS', 'Typed requests and responses'],
  },
  python: {
    label: 'Python',
    install: `pip install ${SDK_PACKAGES.PYTHON}`,
    code: `import os
from levelup import LevelUp

levelup = LevelUp(api_key=os.environ["LEVELUP_API_KEY"])

# Report what a player did; your rules decide the rewards.
result = levelup.activities.send(
    event_id="order_789",            # your id: retries never double-award
    event_type="purchase_completed",
    player_external_id="user_123",   # the player's id in your system
    properties={"amount": 99.99, "product_id": "prod_456"},
)

print(result.status)  # "pending": rules run asynchronously`,
    notes: ['Server-side Python', 'Same field names as the REST API'],
  },
};

const SDKSection = () => {
  const [activeSdk, setActiveSdk] = useState<Sdk>('typescript');
  const [copied, setCopied] = useState<'install' | 'code' | null>(null);

  const current = sdks[activeSdk];

  const copy = async (what: 'install' | 'code') => {
    await navigator.clipboard.writeText(what === 'install' ? current.install : current.code);
    setCopied(what);
    setTimeout(() => setCopied(null), 2000);
  };

  return (
    <section id="sdks" className="py-24 relative overflow-hidden">
      <div className="container mx-auto px-4">
        <div className="grid lg:grid-cols-[2fr_3fr] gap-12 items-center max-w-6xl mx-auto">
          {/* Copy */}
          <div>
            <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-purple/10 border border-purple/20 mb-6">
              <Package className="w-4 h-4 text-purple" />
              <span className="text-sm text-purple font-medium">Official SDKs</span>
            </div>
            <h2 className="text-3xl sm:text-4xl font-display font-bold mb-6">
              TypeScript and Python, <span className="text-gradient-primary">ready to go</span>
            </h2>
            <p className="text-lg text-muted-foreground mb-6">
              Official server-side SDKs wrap the REST API with typed methods, authentication and error handling.
              Prefer raw HTTP? Every call is plain JSON, as shown above.
            </p>
            <ul className="space-y-3">
              {current.notes.map((note) => (
                <li key={note} className="flex items-center gap-3 text-muted-foreground">
                  <Check className="w-5 h-5 text-cyan flex-shrink-0" />
                  <span>{note}</span>
                </li>
              ))}
              <li className="flex items-center gap-3 text-muted-foreground">
                <Check className="w-5 h-5 text-cyan flex-shrink-0" />
                <span>Keep API keys on your server, never in a browser or app</span>
              </li>
            </ul>
          </div>

          {/* Code */}
          <div className="rounded-2xl overflow-hidden border border-border/50 bg-[#0d1117] shadow-2xl min-w-0">
            <div className="flex items-center justify-between px-4 py-3 bg-[#161b22] border-b border-border/30">
              <div className="flex gap-1">
                {(Object.keys(sdks) as Sdk[]).map((sdk) => (
                  <button
                    key={sdk}
                    onClick={() => setActiveSdk(sdk)}
                    className={`px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
                      activeSdk === sdk
                        ? 'bg-cyan/20 text-cyan'
                        : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
                    }`}
                  >
                    {sdks[sdk].label}
                  </button>
                ))}
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => copy('code')}
                className="text-muted-foreground hover:text-foreground"
              >
                {copied === 'code' ? <Check className="w-4 h-4 text-green-500" /> : <Copy className="w-4 h-4" />}
                <span className="ml-2">{copied === 'code' ? 'Copied!' : 'Copy'}</span>
              </Button>
            </div>

            {/* Install */}
            <button
              type="button"
              onClick={() => copy('install')}
              className="w-full flex items-center justify-between gap-3 px-6 py-3 border-b border-border/20 bg-[#161b22]/50 text-left"
              aria-label={`Copy install command: ${current.install}`}
            >
              <span className="flex items-center gap-3 font-mono text-sm text-foreground/90">
                <Terminal className="w-4 h-4 text-cyan" />
                {current.install}
              </span>
              {copied === 'install' ? (
                <Check className="w-4 h-4 text-green-500" />
              ) : (
                <Copy className="w-4 h-4 text-muted-foreground" />
              )}
            </button>

            <div className="p-6 overflow-x-auto">
              <pre className="text-sm leading-relaxed">
                <code className="text-foreground/90 font-mono">{current.code}</code>
              </pre>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default SDKSection;
