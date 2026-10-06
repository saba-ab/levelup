import type { Metadata } from "next";
import ContentPage, { BulletList, Strong, TextLink } from "@/components/ContentPage";
import { CONTACT_EMAIL, PORTAL_ROUTES } from "@/lib/constants";

export const metadata: Metadata = {
  title: "About - LevelUpOS",
  description: "LevelUpOS is a multi-tenant gamification API: points, XP, badges, missions, streaks, rewards and leaderboards driven by your product's activity.",
};

const sections = [
  {
    id: "what-we-build",
    title: "What we build",
    body: (
      <>
        <p>
          LevelUpOS is a gamification engine delivered as an API. Businesses (we call them{" "}
          <Strong>tenants</Strong>) connect their own product to it: a shop, a learning platform, a game or a SaaS
          app. Their end users become <Strong>players</Strong>, identified by the tenant&apos;s own ids.
        </p>
        <p>
          The tenant&apos;s backend reports what players do as <Strong>activities</Strong> (for example{" "}
          <code className="font-mono text-sm text-foreground">purchase_completed</code> with an amount). Rules the
          tenant defines in the portal decide what each activity is worth, and the platform keeps the results:
        </p>
        <BulletList
          items={[
            "Points wallets backed by an append-only ledger",
            "Experience points (XP) and levels, with optional level-up rewards",
            "Badges in tiers from bronze to diamond",
            "One-time, daily, weekly and repeating missions",
            "Streaks counted per day or period in the tenant's timezone",
            "A rewards catalogue players can claim with points",
            "Leaderboards by points, XP, badges or missions, with daily, weekly, monthly or all-time periods",
          ]}
        />
      </>
    ),
  },
  {
    id: "how-it-works",
    title: "How it works",
    body: (
      <>
        <p>
          Activities are accepted immediately (HTTP 202) and evaluated asynchronously by the rules engine. Every
          effect a rule produces, such as crediting points or awarding a badge, is applied exactly once: retrying a
          request with the same <code className="font-mono text-sm text-foreground">event_id</code> or{" "}
          <code className="font-mono text-sm text-foreground">Idempotency-Key</code> never awards twice.
        </p>
        <p>
          Tenants manage everything from the LevelUp portal: event types, rules (with a simulator that previews a
          decision without side effects), the mechanics above, players and API keys. Integrations use the REST API
          directly or the official TypeScript and Python SDKs.
        </p>
      </>
    ),
  },
  {
    id: "principles",
    title: "What we care about",
    body: (
      <BulletList
        items={[
          <>
            <Strong>Correct balances.</Strong> Points, XP and badge awards are ledgers, not counters, and scheduled
            reconciliation checks that totals match.
          </>,
          <>
            <Strong>Tenant isolation.</Strong> Every request and every background job carries its tenant, and one
            tenant can never read another&apos;s data.
          </>,
          <>
            <Strong>Plain, documented APIs.</Strong> JSON over HTTPS with stable error codes, so any language can
            integrate.
          </>,
        ]}
      />
    ),
  },
  {
    id: "where-we-run",
    title: "Where it runs",
    body: (
      <p>
        The API and its databases run on a server operated by Hetzner in Nuremberg, Germany. This website and the
        portal are hosted on Vercel. See <TextLink href="/security">Security</TextLink> and{" "}
        <TextLink href="/privacy">Privacy</TextLink> for details.
      </p>
    ),
  },
  {
    id: "contact",
    title: "Get in touch",
    body: (
      <p>
        Questions, feedback or partnership ideas: <TextLink href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</TextLink>.
        Ready to try it? <TextLink href={PORTAL_ROUTES.SIGNUP}>Create a free account</TextLink>.
      </p>
    ),
  },
];

export default function AboutPage() {
  return (
    <ContentPage
      eyebrow="About LevelUpOS"
      title={
        <>
          Gamification as <span className="text-gradient-primary">infrastructure</span>
        </>
      }
      intro={
        <p>
          We build the engine behind points, levels, badges and missions so product teams can reward what their
          users do without building and maintaining that machinery themselves.
        </p>
      }
      sections={sections}
    />
  );
}
