import { Building2, Settings, Plug } from "lucide-react";

const steps = [
  {
    number: "01",
    icon: Building2,
    title: "Sign Up & Create Your Organization",
    description: "Get started in minutes with our multi-tenant architecture. Each organization gets complete data isolation and customization.",
    color: "cyan",
  },
  {
    number: "02",
    icon: Settings,
    title: "Configure Your Gamification Mechanics",
    description: "Set up badges, levels, missions, and rewards through our intuitive API. Define rules that match your engagement goals.",
    color: "purple",
  },
  {
    number: "03",
    icon: Plug,
    title: "Integrate & Automate",
    description: "Connect your app via our REST API. The rules engine automatically triggers rewards, badges, and level-ups based on user actions.",
    color: "gold",
  },
];

const HowItWorksSection = () => {
  return (
    <section id="how-it-works" className="py-24 relative overflow-hidden">
      <div className="container mx-auto px-4">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-purple/10 border border-purple/20 mb-6">
            <Settings className="w-4 h-4 text-purple" />
            <span className="text-sm text-purple font-medium">Simple Integration</span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-display font-bold mb-6">
            Get Started in{" "}
            <span className="text-gradient-gold">Three Steps</span>
          </h2>
          <p className="text-lg text-muted-foreground">
            From sign-up to fully automated gamification in minutes, not months.
          </p>
        </div>

        {/* Steps */}
        <div className="relative max-w-5xl mx-auto">
          {/* Connection Line */}
          <div className="absolute top-24 left-1/2 -translate-x-1/2 w-[80%] h-0.5 bg-gradient-to-r from-cyan via-purple to-gold hidden lg:block" />

          <div className="grid lg:grid-cols-3 gap-8">
            {steps.map((step, index) => (
              <div key={step.number} className="relative">
                {/* Card */}
                <div className="bg-gradient-card rounded-2xl border border-border/50 p-8 text-center card-shadow hover:border-cyan/30 transition-all duration-300 group">
                  {/* Number Badge */}
                  <div className={`
                    absolute -top-4 left-1/2 -translate-x-1/2 w-8 h-8 rounded-full 
                    flex items-center justify-center text-sm font-bold
                    ${step.color === 'cyan' ? 'bg-cyan text-primary-foreground' : ''}
                    ${step.color === 'purple' ? 'bg-purple text-primary-foreground' : ''}
                    ${step.color === 'gold' ? 'bg-gold text-primary-foreground' : ''}
                  `}>
                    {index + 1}
                  </div>

                  {/* Icon */}
                  <div className={`
                    w-16 h-16 rounded-2xl mx-auto mb-6 flex items-center justify-center
                    group-hover:scale-110 transition-transform
                    ${step.color === 'cyan' ? 'bg-cyan/10' : ''}
                    ${step.color === 'purple' ? 'bg-purple/10' : ''}
                    ${step.color === 'gold' ? 'bg-gold/10' : ''}
                  `}>
                    <step.icon className={`
                      w-8 h-8
                      ${step.color === 'cyan' ? 'text-cyan' : ''}
                      ${step.color === 'purple' ? 'text-purple' : ''}
                      ${step.color === 'gold' ? 'text-gold' : ''}
                    `} />
                  </div>

                  {/* Content */}
                  <h3 className="text-xl font-display font-semibold text-foreground mb-3">
                    {step.title}
                  </h3>
                  <p className="text-muted-foreground text-sm leading-relaxed">
                    {step.description}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
};

export default HowItWorksSection;
