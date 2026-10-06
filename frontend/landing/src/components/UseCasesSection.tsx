import { ShoppingCart, Laptop, Smartphone, GraduationCap } from "lucide-react";

const useCases = [
  {
    icon: ShoppingCart,
    title: "E-commerce",
    description: "Reward purchases, track loyalty points, offer exclusive badges for your best customers.",
    examples: ["Purchase rewards", "Loyalty tiers", "Referral bonuses", "VIP badges"],
    gradient: "from-cyan/20 to-cyan/5",
    borderColor: "hover:border-cyan/40",
  },
  {
    icon: Laptop,
    title: "SaaS Platforms",
    description: "Increase user engagement with missions, achievement systems, and progress tracking.",
    examples: ["Feature adoption", "Onboarding quests", "Power user badges", "Usage streaks"],
    gradient: "from-purple/20 to-purple/5",
    borderColor: "hover:border-purple/40",
  },
  {
    icon: Smartphone,
    title: "Mobile Apps",
    description: "Drive daily logins with streaks, level progression, and engaging challenges.",
    examples: ["Daily check-ins", "Level progression", "Achievement unlocks", "Leaderboards"],
    gradient: "from-gold/20 to-gold/5",
    borderColor: "hover:border-gold/40",
  },
  {
    icon: GraduationCap,
    title: "Education",
    description: "Gamify learning with XP, badges, and leaderboards that motivate students.",
    examples: ["Course completion", "Quiz rewards", "Learning streaks", "Skill badges"],
    gradient: "from-cyan/20 to-cyan/5",
    borderColor: "hover:border-cyan/40",
  },
];

const UseCasesSection = () => {
  return (
    <section id="use-cases" className="py-24 relative overflow-hidden">
      <div className="container mx-auto px-4">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-cyan/10 border border-cyan/20 mb-6">
            <Laptop className="w-4 h-4 text-cyan" />
            <span className="text-sm text-cyan font-medium">Use Cases</span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-display font-bold mb-6">
            Built for{" "}
            <span className="text-gradient-gold">Every Industry</span>
          </h2>
          <p className="text-lg text-muted-foreground">
            From e-commerce to education, LevelUpOS powers engagement across all platforms.
          </p>
        </div>

        {/* Use Cases Grid */}
        <div className="grid md:grid-cols-2 gap-6 max-w-5xl mx-auto">
          {useCases.map((useCase) => (
            <div
              key={useCase.title}
              className={`
                relative overflow-hidden rounded-2xl border border-border/50 
                bg-gradient-to-br ${useCase.gradient} bg-card
                p-8 transition-all duration-300 ${useCase.borderColor} group card-shadow
              `}
            >
              {/* Icon */}
              <div className="w-14 h-14 rounded-xl bg-background/50 flex items-center justify-center mb-6 group-hover:scale-110 transition-transform">
                <useCase.icon className="w-7 h-7 text-foreground" />
              </div>

              {/* Content */}
              <h3 className="text-2xl font-display font-semibold text-foreground mb-3">
                {useCase.title}
              </h3>
              <p className="text-muted-foreground mb-6 leading-relaxed">
                {useCase.description}
              </p>

              {/* Examples */}
              <div className="flex flex-wrap gap-2">
                {useCase.examples.map((example) => (
                  <span
                    key={example}
                    className="px-3 py-1 rounded-full bg-background/50 text-sm text-muted-foreground border border-border/50"
                  >
                    {example}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default UseCasesSection;
