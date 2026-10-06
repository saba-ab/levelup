import type { Metadata } from "next";
import ContentPage, { BulletList, Placeholder, Strong, TextLink } from "@/components/ContentPage";
import { LEGAL } from "@/lib/constants";

export const metadata: Metadata = {
  title: "Privacy Policy - LevelUpOS",
  description: "What data LevelUpOS processes, why, where it is stored and who it is shared with.",
};

const sections = [
  {
    id: "who-we-are",
    title: "Who we are",
    body: (
      <>
        <p>
          LevelUpOS (&quot;LevelUp&quot;, &quot;we&quot;) is operated by <Placeholder>{LEGAL.COMPANY_NAME}</Placeholder>,{" "}
          <Placeholder>{LEGAL.COMPANY_ADDRESS}</Placeholder>, registration number{" "}
          <Placeholder>{LEGAL.COMPANY_REGISTRATION}</Placeholder>.
        </p>
        <p>
          This policy covers this website, the LevelUp portal and the LevelUp API. Privacy questions:{" "}
          <Placeholder>{LEGAL.PRIVACY_CONTACT}</Placeholder>.
        </p>
      </>
    ),
  },
  {
    id: "roles",
    title: "Our role",
    body: (
      <>
        <p>
          LevelUp is used by businesses (&quot;tenants&quot;) to add gamification to their own products. We act in
          two roles:
        </p>
        <BulletList
          items={[
            <>
              <Strong>Controller</Strong> for the accounts of people who sign up for and use the portal, and for data
              about how our service is used.
            </>,
            <>
              <Strong>Processor</Strong> for the player data a tenant sends us. The tenant decides what to send and
              why, and is responsible for having a lawful basis for it. If you are a player in a tenant&apos;s app,
              please contact that business first; we will help them answer your request.
            </>,
          ]}
        />
      </>
    ),
  },
  {
    id: "data-we-process",
    title: "Data we process",
    body: (
      <>
        <p>
          <Strong>Portal accounts.</Strong> Your name, email address, the organization (tenant) you belong to and
          your roles. Passwords are never stored in readable form: we keep only a bcrypt hash.
        </p>
        <p>
          <Strong>API keys.</Strong> We store a short display prefix and a SHA-256 hash of each key, never the key
          itself. The full key is shown once, when it is created.
        </p>
        <p>
          <Strong>Player data sent by tenants.</Strong> Tenants send activities about their players. An activity
          contains:
        </p>
        <BulletList
          items={[
            "the player's id in the tenant's system (external id)",
            "an event type, such as purchase_completed",
            "the time it happened and any properties the tenant chooses to include",
          ]}
        />
        <p>From those activities the platform stores, per player:</p>
        <BulletList
          items={[
            "points wallets and their transaction ledgers",
            "XP and levels",
            "badges",
            "mission progress, streaks and reward claims",
            "leaderboard standings",
            "the rule decisions that produced them",
          ]}
        />
        <p>
          We ask tenants not to send names, contact details or special categories of personal data in activity
          properties unless their use case needs them.
        </p>
        <p>
          <Strong>Technical data.</Strong> IP addresses and request metadata are used for rate limiting, security
          and operational logs.
        </p>
        <p>
          <Strong>AI drafting (optional).</Strong> If you use the portal&apos;s AI drafting features, the prompt
          you write and the context needed to answer it are sent to Anthropic to generate a draft. Nothing is sent
          to Anthropic unless you use these features.
        </p>
      </>
    ),
  },
  {
    id: "purposes",
    title: "Why we process it",
    body: (
      <BulletList
        items={[
          "To provide the service: authenticate you, run your rules and store the results (performance of a contract).",
          "To send transactional email such as password resets and invitations (performance of a contract).",
          "To keep the service secure and available: rate limiting, abuse prevention, logs and backups (legitimate interests).",
          "To meet legal obligations, where they apply.",
        ]}
      />
    ),
  },
  {
    id: "where-stored",
    title: "Where data is stored",
    body: (
      <>
        <p>
          The API, its databases, cache and message queue run on a server operated by Hetzner Online GmbH in
          Nuremberg, Germany. Tenant and player data is stored there.
        </p>
        <p>
          This website and the portal are web applications hosted by Vercel. They run in your browser and talk to
          the API over HTTPS.
        </p>
      </>
    ),
  },
  {
    id: "subprocessors",
    title: "Service providers",
    body: (
      <>
        <p>We use these providers to run LevelUp:</p>
        <BulletList
          items={[
            <>
              <Strong>Hetzner Online GmbH</Strong> (Germany): hosting of the API and databases.
            </>,
            <>
              <Strong>Vercel Inc.</Strong>: hosting of this website and the portal.
            </>,
            <>
              <Strong>Anthropic</Strong>: AI drafting, only when you use it.
            </>,
            <>
              <Placeholder>{LEGAL.EMAIL_PROVIDER}</Placeholder>: delivery of transactional email.
            </>,
          ]}
        />
        <p>We do not sell personal data and do not use it for advertising.</p>
      </>
    ),
  },
  {
    id: "cookies",
    title: "Cookies and browser storage",
    body: (
      <p>
        This website does not use advertising or analytics cookies. The portal stores your sign-in session in your
        browser so you stay logged in, plus interface preferences such as the theme.
      </p>
    ),
  },
  {
    id: "retention",
    title: "How long we keep data",
    body: (
      <>
        <p>
          We keep account and tenant data while the tenant&apos;s account is active. When a tenant is deleted, its
          data is purged from every part of the platform.
        </p>
        <p>
          Tenants can deactivate or delete individual players through the API or portal. Database backups are kept
          for <Placeholder>{LEGAL.BACKUP_RETENTION}</Placeholder> and then overwritten.
        </p>
      </>
    ),
  },
  {
    id: "security",
    title: "Security",
    body: (
      <p>
        All traffic is encrypted with TLS, passwords and API keys are stored only as hashes, and tenants are
        isolated from each other. Read more on our <TextLink href="/security">Security page</TextLink>.
      </p>
    ),
  },
  {
    id: "your-rights",
    title: "Your rights",
    body: (
      <>
        <p>
          Depending on where you live, you may have the right to access, correct, delete, restrict or export your
          personal data, and to object to processing. To exercise them, contact{" "}
          <Placeholder>{LEGAL.PRIVACY_CONTACT}</Placeholder>.
        </p>
        <p>
          You may also complain to a data protection authority, such as{" "}
          <Placeholder>{LEGAL.SUPERVISORY_AUTHORITY}</Placeholder>.
        </p>
      </>
    ),
  },
  {
    id: "changes",
    title: "Changes to this policy",
    body: (
      <p>
        We will update this page when our practices change and adjust the date at the top. For significant
        changes we will notify account owners by email.
      </p>
    ),
  },
];

export default function PrivacyPage() {
  return (
    <ContentPage
      eyebrow="Legal"
      title="Privacy Policy"
      intro={
        <p>
          What data LevelUp processes, why, where it is stored and who it is shared with.
        </p>
      }
      lastUpdated={LEGAL.LAST_UPDATED}
      sections={sections}
    />
  );
}
