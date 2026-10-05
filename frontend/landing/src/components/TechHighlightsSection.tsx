import { Server, Shield, Zap, FileCode, Users, Activity } from "lucide-react";

const highlights = [
  {
    icon: Server,
    label: "RESTful API",
  },
  {
    icon: Shield,
    label: "Data Isolation",
  },
  {
    icon: Zap,
    label: "Real-time Events",
  },
  {
    icon: FileCode,
    label: "Full Documentation",
  },
  {
    icon: Users,
    label: "Multi-tenant",
  },
  {
    icon: Activity,
    label: "99.9% Uptime",
  },
];

const stats = [
  { value: "500+", label: "Organizations" },
  { value: "10M+", label: "API Calls/Day" },
  { value: "50M+", label: "Badges Awarded" },
  { value: "99.9%", label: "Uptime SLA" },
];

const TechHighlightsSection = () => {
  return (
    <section className="py-24 relative overflow-hidden bg-secondary/30">
      <div className="container mx-auto px-4">
        {/* Technical Highlights */}
        <div className="text-center mb-12">
          <h2 className="text-2xl sm:text-3xl font-display font-bold mb-4">
            Enterprise-Grade <span className="text-gradient-primary">Infrastructure</span>
          </h2>
          <p className="text-muted-foreground max-w-xl mx-auto">
            Built with reliability, security, and developer experience in mind.
          </p>
        </div>

        {/* Tech Pills */}
        <div className="flex flex-wrap justify-center gap-4 mb-16">
          {highlights.map((highlight) => (
            <div
              key={highlight.label}
              className="flex items-center gap-2 px-5 py-3 rounded-full bg-card border border-border/50 hover:border-cyan/30 transition-colors"
            >
              <highlight.icon className="w-5 h-5 text-cyan" />
              <span className="text-sm font-medium text-foreground">{highlight.label}</span>
            </div>
          ))}
        </div>

        {/* Stats */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-6 max-w-4xl mx-auto">
          {stats.map((stat) => (
            <div
              key={stat.label}
              className="text-center p-6 rounded-2xl bg-gradient-card border border-border/50"
            >
              <div className="text-3xl sm:text-4xl font-display font-bold text-gradient-primary mb-2">
                {stat.value}
              </div>
              <div className="text-sm text-muted-foreground">{stat.label}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default TechHighlightsSection;
