'use client'

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Check, Sparkles, Zap, Building2 } from "lucide-react";
import { PORTAL_ROUTES } from "@/lib/constants";

const pricingTiers = [
  {
    name: "Free",
    icon: Zap,
    description: "Perfect for getting started and exploring the platform.",
    monthlyPrice: 0,
    yearlyPrice: 0,
    features: [
      "Up to 1,000 active users",
      "5 badge types",
      "3 missions",
      "Basic leaderboards",
      "Community support",
      "7-day data retention",
    ],
    cta: "Get Started Free",
    variant: "heroOutline" as const,
    popular: false,
  },
  {
    name: "Pro",
    icon: Sparkles,
    description: "For growing teams ready to scale their gamification.",
    monthlyPrice: 49,
    yearlyPrice: 39,
    features: [
      "Up to 50,000 active users",
      "Unlimited badges & tiers",
      "Unlimited missions",
      "Advanced leaderboards",
      "Rules engine automation",
      "Priority email support",
      "90-day data retention",
      "Webhook integrations",
    ],
    cta: "Start Pro Trial",
    variant: "hero" as const,
    popular: true,
  },
  {
    name: "Enterprise",
    icon: Building2,
    description: "Custom solutions for large-scale deployments.",
    monthlyPrice: null,
    yearlyPrice: null,
    features: [
      "Unlimited active users",
      "Custom badge designs",
      "White-label options",
      "Dedicated account manager",
      "24/7 phone & email support",
      "Unlimited data retention",
      "Custom integrations",
      "SLA guarantee",
      "On-premise deployment",
    ],
    cta: "Contact Sales",
    variant: "heroOutline" as const,
    popular: false,
  },
];

const PricingSection = () => {
  const [isYearly, setIsYearly] = useState(false);

  return (
    <section id="pricing" className="py-24 relative overflow-hidden">
      {/* Background */}
      <div className="absolute inset-0 bg-gradient-to-b from-secondary/30 via-background to-background" />

      <div className="container mx-auto px-4 relative z-10">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-12">
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-gold/10 border border-gold/20 mb-6">
            <Sparkles className="w-4 h-4 text-gold" />
            <span className="text-sm text-gold font-medium">Simple Pricing</span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-display font-bold mb-6">
            Choose Your{" "}
            <span className="text-gradient-primary">Perfect Plan</span>
          </h2>
          <p className="text-lg text-muted-foreground">
            Start free and scale as you grow. No hidden fees, cancel anytime.
          </p>
        </div>

        {/* Billing Toggle */}
        <div className="flex items-center justify-center gap-4 mb-12">
          <span className={`text-sm font-medium ${!isYearly ? 'text-foreground' : 'text-muted-foreground'}`}>
            Monthly
          </span>
          <button
            onClick={() => setIsYearly(!isYearly)}
            className={`
              relative w-14 h-7 rounded-full transition-colors duration-300
              ${isYearly ? 'bg-cyan' : 'bg-secondary'}
            `}
          >
            <div
              className={`
                absolute top-1 w-5 h-5 rounded-full bg-foreground transition-transform duration-300
                ${isYearly ? 'translate-x-8' : 'translate-x-1'}
              `}
            />
          </button>
          <span className={`text-sm font-medium ${isYearly ? 'text-foreground' : 'text-muted-foreground'}`}>
            Yearly
          </span>
          {isYearly && (
            <span className="px-2 py-1 rounded-full bg-gold/10 text-gold text-xs font-medium">
              Save 20%
            </span>
          )}
        </div>

        {/* Pricing Cards */}
        <div className="grid lg:grid-cols-3 gap-8 max-w-6xl mx-auto">
          {pricingTiers.map((tier) => (
            <div
              key={tier.name}
              className={`
                relative bg-gradient-card rounded-2xl border p-8 transition-all duration-300 card-shadow
                ${tier.popular
                  ? 'border-cyan/50 scale-105 lg:scale-110'
                  : 'border-border/50 hover:border-cyan/30'
                }
              `}
            >
              {/* Popular Badge */}
              {tier.popular && (
                <div className="absolute -top-4 left-1/2 -translate-x-1/2 px-4 py-1 rounded-full bg-gradient-to-r from-cyan to-purple text-primary-foreground text-sm font-semibold">
                  Most Popular
                </div>
              )}

              {/* Header */}
              <div className="text-center mb-8">
                <div className={`
                  w-12 h-12 rounded-xl mx-auto mb-4 flex items-center justify-center
                  ${tier.popular ? 'bg-cyan/20' : 'bg-secondary/50'}
                `}>
                  <tier.icon className={`w-6 h-6 ${tier.popular ? 'text-cyan' : 'text-foreground'}`} />
                </div>
                <h3 className="text-2xl font-display font-bold text-foreground mb-2">
                  {tier.name}
                </h3>
                <p className="text-sm text-muted-foreground">
                  {tier.description}
                </p>
              </div>

              {/* Price */}
              <div className="text-center mb-8">
                {tier.monthlyPrice !== null ? (
                  <>
                    <div className="flex items-baseline justify-center gap-1">
                      <span className="text-4xl font-display font-bold text-foreground">
                        ${isYearly ? tier.yearlyPrice : tier.monthlyPrice}
                      </span>
                      <span className="text-muted-foreground">/mo</span>
                    </div>
                    {isYearly && tier.monthlyPrice > 0 && (
                      <p className="text-sm text-muted-foreground mt-1">
                        Billed annually (${tier.yearlyPrice! * 12}/year)
                      </p>
                    )}
                  </>
                ) : (
                  <div className="text-3xl font-display font-bold text-foreground">
                    Custom
                  </div>
                )}
              </div>

              {/* Features */}
              <ul className="space-y-3 mb-8">
                {tier.features.map((feature) => (
                  <li key={feature} className="flex items-start gap-3">
                    <Check className={`w-5 h-5 flex-shrink-0 mt-0.5 ${tier.popular ? 'text-cyan' : 'text-gold'}`} />
                    <span className="text-sm text-muted-foreground">{feature}</span>
                  </li>
                ))}
              </ul>

              {/* CTA */}
              <Button variant={tier.variant} size="lg" className="w-full" asChild>
                <a href={tier.name === 'Enterprise' ? 'mailto:sales@levelupos.com' : PORTAL_ROUTES.SIGNUP}>
                  {tier.cta}
                </a>
              </Button>
            </div>
          ))}
        </div>

        {/* Bottom Note */}
        <p className="text-center text-sm text-muted-foreground mt-12">
          All plans include API access, documentation, and basic analytics. Need something custom?{" "}
          <a href="mailto:sales@levelupos.com" className="text-cyan hover:underline">Let&apos;s talk</a>.
        </p>
      </div>
    </section>
  );
};

export default PricingSection;
