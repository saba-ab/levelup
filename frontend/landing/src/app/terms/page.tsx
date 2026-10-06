import type { Metadata } from "next";
import ContentPage, { BulletList, Placeholder, Strong, TextLink } from "@/components/ContentPage";
import { LEGAL } from "@/lib/constants";

export const metadata: Metadata = {
  title: "Terms of Service - LevelUpOS",
  description: "The terms that apply when you use the LevelUpOS website, portal and API.",
};

const sections = [
  {
    id: "agreement",
    title: "Agreement",
    body: (
      <p>
        These terms apply between you and <Placeholder>{LEGAL.COMPANY_NAME}</Placeholder> (&quot;LevelUp&quot;,
        &quot;we&quot;) when you use the LevelUpOS website, portal or API (the &quot;Service&quot;). If you use the
        Service on behalf of an organization, you accept them for that organization and confirm you are authorized
        to do so.
      </p>
    ),
  },
  {
    id: "service",
    title: "The Service",
    body: (
      <>
        <p>
          LevelUp is a multi-tenant gamification API. Your systems send player activity to it, and it stores and
          computes points ledgers, XP and levels, badges, missions, streaks, reward claims and leaderboards according
          to rules you configure.
        </p>
        <p>
          We may change, add or remove features over time. When a change removes functionality you rely on, we
          will try to give reasonable notice.
        </p>
      </>
    ),
  },
  {
    id: "accounts",
    title: "Accounts and API keys",
    body: (
      <BulletList
        items={[
          "You must give accurate registration details and keep your password confidential.",
          "API keys are credentials. Keep them on your servers, never in browsers or mobile apps, and revoke any key you believe is exposed.",
          "You are responsible for activity under your account and keys, including that of users you invite.",
        ]}
      />
    ),
  },
  {
    id: "your-data",
    title: "Your data",
    body: (
      <>
        <p>
          You keep all rights to the data you send to the Service (&quot;Customer Data&quot;). You grant us the
          rights needed to host, process and display it in order to provide the Service to you.
        </p>
        <p>
          You are responsible for the lawfulness of Customer Data, including having a valid basis and giving any
          notices required to send us data about your players. We process player data on your behalf as described
          in our <TextLink href="/privacy">Privacy Policy</TextLink>.
        </p>
        <p>
          Points, badges, levels and rewards in the Service are records you define for your own program. They have
          no cash value from LevelUp, and you are responsible for any promise you make to your players about them.
        </p>
      </>
    ),
  },
  {
    id: "acceptable-use",
    title: "Acceptable use",
    body: (
      <>
        <p>You agree not to:</p>
        <BulletList
          items={[
            "use the Service for anything unlawful, or to store data you have no right to process;",
            "attempt to access another tenant's data or bypass authentication, authorization or rate limits;",
            "probe, scan or load-test the Service without our written permission;",
            "send malware, or use the Service to harm others;",
            "resell the Service without our agreement.",
          ]}
        />
        <p>We may suspend access that puts the Service or other tenants at risk, and will tell you why.</p>
      </>
    ),
  },
  {
    id: "ai-features",
    title: "AI drafting",
    body: (
      <p>
        Optional AI drafting features in the portal send your prompt to Anthropic to generate a draft. Drafts can be
        wrong. Review them before you publish rules or other content based on them.
      </p>
    ),
  },
  {
    id: "fees",
    title: "Plans and fees",
    body: (
      <p>
        Plans and prices are shown on our <TextLink href="/#pricing">pricing section</TextLink>. If you choose a
        paid plan, the price and billing terms shown at the time of purchase apply.
      </p>
    ),
  },
  {
    id: "availability",
    title: "Availability",
    body: (
      <p>
        We work to keep the Service available and your data safe, but unless a separate written agreement says
        otherwise, the Service is provided &quot;as is&quot; and &quot;as available&quot;, without uptime or
        performance commitments.
      </p>
    ),
  },
  {
    id: "liability",
    title: "Limitation of liability",
    body: (
      <p>
        To the extent permitted by law, LevelUp is not liable for indirect or consequential losses, or for loss of
        profits, revenue or data, arising from your use of the Service. Nothing in these terms limits liability that
        cannot be limited by law.
      </p>
    ),
  },
  {
    id: "termination",
    title: "Ending your use",
    body: (
      <>
        <p>
          You may stop using the Service at any time and ask us to delete your tenant. Deleting a tenant purges its
          data from the platform.
        </p>
        <p>
          We may end or suspend your access if you materially breach these terms. Where we can, we will give you
          notice and a chance to export your data first.
        </p>
      </>
    ),
  },
  {
    id: "law",
    title: "Governing law",
    body: (
      <p>
        These terms are governed by <Placeholder>{LEGAL.GOVERNING_LAW}</Placeholder>.
      </p>
    ),
  },
  {
    id: "changes",
    title: "Changes and contact",
    body: (
      <>
        <p>
          We may update these terms. We will change the date at the top and notify account owners by email of
          material changes before they take effect.
        </p>
        <p>
          <Strong>Contact:</Strong> <Placeholder>{LEGAL.COMPANY_NAME}</Placeholder>,{" "}
          <Placeholder>{LEGAL.COMPANY_ADDRESS}</Placeholder>.
        </p>
      </>
    ),
  },
];

export default function TermsPage() {
  return (
    <ContentPage
      eyebrow="Legal"
      title="Terms of Service"
      intro={<p>The terms that apply when you use the LevelUpOS website, portal and API.</p>}
      lastUpdated={LEGAL.LAST_UPDATED}
      sections={sections}
    />
  );
}
