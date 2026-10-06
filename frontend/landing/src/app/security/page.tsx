import type { Metadata } from "next";
import ContentPage, { BulletList, Placeholder, Strong } from "@/components/ContentPage";
import { LEGAL } from "@/lib/constants";

export const metadata: Metadata = {
  title: "Security - LevelUpOS",
  description: "The controls LevelUpOS uses to protect tenant data: isolation, hashed credentials, idempotency, rate limiting, encryption in transit and backups.",
};

const code = "font-mono text-sm text-foreground";

const sections = [
  {
    id: "tenant-isolation",
    title: "Tenant isolation",
    body: (
      <BulletList
        items={[
          "Every request is authenticated, and the tenant comes from the verified credential, never from the request body.",
          "Every query on tenant-owned data filters by that tenant. Asking for another tenant's resource returns 404, the same as a resource that does not exist.",
          "Background work (rule evaluation, awards, leaderboard updates) carries the tenant id in every message. Handlers never infer it.",
          "Access inside a tenant is role-based: admins, program managers and developers get different permissions.",
        ]}
      />
    ),
  },
  {
    id: "credentials",
    title: "Passwords, sessions and API keys",
    body: (
      <>
        <p>
          <Strong>Passwords</Strong> are hashed with bcrypt. Portal sessions use access tokens that expire after 15
          minutes, with rotating refresh tokens. Logging out revokes them.
        </p>
        <p>
          <Strong>API keys</Strong> for your backends look like <code className={code}>lvl_live_…</code> and carry a
          160-bit random secret.
        </p>
        <BulletList
          items={[
            "We store only a SHA-256 hash of each key and compare in constant time. The full key is shown once, at creation.",
            "Revoking a key takes effect on the next request.",
            "A key never holds a role senior to the person who created it.",
            "Keys cannot create other keys, manage users, assign roles or change tenant settings. Those actions require a signed-in person, so a leaked key cannot mint new access.",
          ]}
        />
      </>
    ),
  },
  {
    id: "integrity",
    title: "Idempotency and data integrity",
    body: (
      <BulletList
        items={[
          <>
            Activities are deduplicated on your <code className={code}>event_id</code>: sending the same activity
            twice produces one decision.
          </>,
          <>
            Endpoints that move points require an <code className={code}>Idempotency-Key</code>, so a retried
            request never credits or debits twice.
          </>,
          "Every effect a rule produces (points, XP, badges, missions, streaks, rewards) is applied exactly once, even when messages are redelivered or arrive out of order.",
          "Points, XP and badge awards are append-only ledgers. Wallet balances cannot go negative, and scheduled reconciliation jobs check that ledgers and balances agree.",
          "Changes and the messages announcing them are written in the same database transaction, so an award is never recorded without its follow-up work (or the other way round).",
        ]}
      />
    ),
  },
  {
    id: "rate-limiting",
    title: "Rate limiting",
    body: (
      <p>
        API requests are rate limited per API key or user, and per IP address for unauthenticated requests. Clients
        over the limit receive <code className={code}>429</code> with a <code className={code}>Retry-After</code>{" "}
        header.
      </p>
    ),
  },
  {
    id: "transport",
    title: "Encryption in transit and network exposure",
    body: (
      <>
        <p>
          All traffic to the API, the portal and this website is served over HTTPS (TLS). The API runs behind a TLS
          reverse proxy; the database, cache and message broker are not reachable from the internet, and metrics
          are only reachable from the server itself.
        </p>
        <p>
          Webhook deliveries are signed, so your endpoint can verify that a request came from LevelUp before acting
          on it.
        </p>
      </>
    ),
  },
  {
    id: "hosting",
    title: "Hosting and backups",
    body: (
      <>
        <p>
          The API and its data run on a Hetzner server in Nuremberg, Germany. The portal and this website are hosted
          on Vercel. Production secrets live only on the server, in a file readable by the administrator account, and
          never in source control.
        </p>
        <p>
          The database is backed up daily. Backups are kept for <Placeholder>{LEGAL.BACKUP_RETENTION}</Placeholder>.
          Schema migrations are forward-only and never drop data on deploy.
        </p>
      </>
    ),
  },
  {
    id: "ai",
    title: "AI features",
    body: (
      <p>
        AI drafting in the portal is optional. When you use it, your prompt and the context needed to answer it are
        sent to Anthropic over TLS. Nothing is sent unless you use the feature.
      </p>
    ),
  },
  {
    id: "disclosure",
    title: "Reporting a vulnerability",
    body: (
      <p>
        If you believe you have found a security issue, please email{" "}
        <Placeholder>{LEGAL.SECURITY_CONTACT}</Placeholder> with steps to reproduce. Please do not access other
        tenants&apos; data or degrade the service while testing. We will acknowledge your report and keep you
        updated.
      </p>
    ),
  },
];

export default function SecurityPage() {
  return (
    <ContentPage
      eyebrow="Trust"
      title="Security"
      intro={
        <p>
          The controls that protect your data in LevelUp, described as they are built today. We do not currently
          hold third-party security certifications.
        </p>
      }
      lastUpdated={LEGAL.LAST_UPDATED}
      sections={sections}
    />
  );
}
