import { 
  Building, 
  Code2, 
  Sparkles, 
  ScrollText, 
  Puzzle,
  CheckCircle2
} from "lucide-react";

const benefits = [
  {
    icon: Building,
    title: "Multi-Tenant Architecture",
    description: "Each organization gets isolated data and complete customization. Scale from startup to enterprise without changing infrastructure.",
    highlights: ["Complete data isolation", "Custom branding per tenant", "Independent configurations"],
  },
  {
    icon: Code2,
    title: "API-First Design",
    description: "RESTful API for seamless integration with any platform. Well-documented endpoints with SDKs for popular languages.",
    highlights: ["RESTful endpoints", "Comprehensive documentation", "SDK support"],
  },
  {
    icon: Sparkles,
    title: "Automated Rules Engine",
    description: "Set up event-driven gamification without writing custom code. Define triggers and the system handles the rest.",
    highlights: ["No-code automation", "Event-driven triggers", "Real-time processing"],
  },
  {
    icon: ScrollText,
    title: "Complete Audit Trail",
    description: "Every transaction, badge award, and action is tracked. Perfect for compliance and analytics.",
    highlights: ["Full event history", "Compliance ready", "Analytics insights"],
  },
  {
    icon: Puzzle,
    title: "Flexible & Extensible",
    description: "Customize every aspect of your gamification program. Extend with webhooks and custom integrations.",
    highlights: ["Webhook support", "Custom integrations", "Unlimited customization"],
  },
];

const BenefitsSection = () => {
  return (
    <section className="py-24 relative overflow-hidden bg-secondary/30">
      <div className="container mx-auto px-4">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-gold/10 border border-gold/20 mb-6">
            <Sparkles className="w-4 h-4 text-gold" />
            <span className="text-sm text-gold font-medium">Why LevelUpOS</span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-display font-bold mb-6">
            Built for{" "}
            <span className="text-gradient-primary">Developers & Teams</span>
          </h2>
          <p className="text-lg text-muted-foreground">
            Enterprise-grade infrastructure with developer-friendly APIs and complete flexibility.
          </p>
        </div>

        {/* Benefits Grid */}
        <div className="grid lg:grid-cols-2 gap-8 max-w-5xl mx-auto">
          {benefits.map((benefit, index) => (
            <div
              key={benefit.title}
              className={`
                relative bg-gradient-card rounded-2xl border border-border/50 p-8 
                hover:border-cyan/30 transition-all duration-300 card-shadow
                ${index === benefits.length - 1 ? 'lg:col-span-2 lg:max-w-2xl lg:mx-auto' : ''}
              `}
            >
              <div className="flex gap-6">
                {/* Icon */}
                <div className="w-14 h-14 rounded-xl bg-cyan/10 flex items-center justify-center flex-shrink-0">
                  <benefit.icon className="w-7 h-7 text-cyan" />
                </div>

                {/* Content */}
                <div className="flex-1">
                  <h3 className="text-xl font-display font-semibold text-foreground mb-3">
                    {benefit.title}
                  </h3>
                  <p className="text-muted-foreground mb-4 leading-relaxed">
                    {benefit.description}
                  </p>
                  
                  {/* Highlights */}
                  <div className="flex flex-wrap gap-2">
                    {benefit.highlights.map((highlight) => (
                      <div
                        key={highlight}
                        className="flex items-center gap-1.5 text-sm text-muted-foreground"
                      >
                        <CheckCircle2 className="w-4 h-4 text-cyan" />
                        <span>{highlight}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default BenefitsSection;
