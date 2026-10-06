import { 
  Coins, 
  Award, 
  TrendingUp, 
  Target, 
  Flame, 
  Gift, 
  BarChart3, 
  Workflow 
} from "lucide-react";

const features = [
  {
    icon: Coins,
    title: "Points & Wallet System",
    description: "Every credit, debit and transfer is an immutable ledger entry. Balances never go negative.",
    color: "text-gold",
    bgColor: "bg-gold/10",
  },
  {
    icon: Award,
    title: "Badges & Achievements",
    description: "Award badges in tiers from bronze to diamond, automatically from rules or directly via the API.",
    color: "text-cyan",
    bgColor: "bg-cyan/10",
  },
  {
    icon: TrendingUp,
    title: "Levels & XP Progression",
    description: "Define a level ladder, grant XP from rules, and attach points or badges to reaching a level.",
    color: "text-purple",
    bgColor: "bg-purple/10",
  },
  {
    icon: Target,
    title: "Missions & Quests",
    description: "One-time, daily, weekly or repeating missions that track progress and reward completion.",
    color: "text-cyan",
    bgColor: "bg-cyan/10",
  },
  {
    icon: Flame,
    title: "Streaks",
    description: "Streaks counted per period in your timezone, with milestone rewards when players keep going.",
    color: "text-gold",
    bgColor: "bg-gold/10",
  },
  {
    icon: Gift,
    title: "Rewards System",
    description: "A rewards catalogue with stock limits: players claim discounts, items or custom rewards with points.",
    color: "text-purple",
    bgColor: "bg-purple/10",
  },
  {
    icon: BarChart3,
    title: "Leaderboards",
    description: "Rank players by points, XP, badges or missions, all-time or reset daily, weekly or monthly.",
    color: "text-gold",
    bgColor: "bg-gold/10",
  },
  {
    icon: Workflow,
    title: "Rules Engine",
    description: "Rules turn activities into points, XP, badges and more. Preview any rule in the simulator first.",
    color: "text-cyan",
    bgColor: "bg-cyan/10",
  },
];

const FeaturesSection = () => {
  return (
    <section id="features" className="py-24 relative overflow-hidden">
      {/* Background */}
      <div className="absolute inset-0 bg-gradient-to-b from-background via-secondary/20 to-background" />
      
      <div className="container mx-auto px-4 relative z-10">
        {/* Section Header */}
        <div className="text-center max-w-3xl mx-auto mb-16">
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-cyan/10 border border-cyan/20 mb-6">
            <Award className="w-4 h-4 text-cyan" />
            <span className="text-sm text-cyan font-medium">Powerful Features</span>
          </div>
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-display font-bold mb-6">
            Everything You Need to{" "}
            <span className="text-gradient-primary">Gamify Your Platform</span>
          </h2>
          <p className="text-lg text-muted-foreground">
            A complete toolkit of gamification mechanics designed to boost engagement, retention, and user satisfaction.
          </p>
        </div>

        {/* Features Grid */}
        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {features.map((feature, index) => (
            <div
              key={feature.title}
              className="group relative bg-gradient-card rounded-2xl border border-border/50 p-6 hover:border-cyan/30 transition-all duration-300 hover:-translate-y-1 card-shadow"
              style={{ animationDelay: `${index * 100}ms` }}
            >
              {/* Icon */}
              <div className={`w-12 h-12 rounded-xl ${feature.bgColor} flex items-center justify-center mb-4 group-hover:scale-110 transition-transform`}>
                <feature.icon className={`w-6 h-6 ${feature.color}`} />
              </div>

              {/* Content */}
              <h3 className="text-lg font-display font-semibold text-foreground mb-2">
                {feature.title}
              </h3>
              <p className="text-sm text-muted-foreground leading-relaxed">
                {feature.description}
              </p>

              {/* Hover glow effect */}
              <div className="absolute inset-0 rounded-2xl bg-gradient-to-br from-cyan/5 to-purple/5 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none" />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default FeaturesSection;
