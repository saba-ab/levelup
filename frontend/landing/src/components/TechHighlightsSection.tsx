import { Server, Shield, Workflow, FileCode, Users, KeyRound } from "lucide-react";

const highlights = [
  { icon: Server, label: "REST API" },
  { icon: Shield, label: "Tenant Isolation" },
  { icon: Workflow, label: "Async Rule Processing" },
  { icon: FileCode, label: "API Reference" },
  { icon: Users, label: "Multi-tenant" },
  { icon: KeyRound, label: "Hashed API Keys" },
];

const guarantees = [
  { value: "202", label: "Activities are accepted at once, then evaluated by your rules" },
  { value: "1×", label: "Each award applied exactly once, even when requests are retried" },
  { value: "≥ 0", label: "Wallet balances never go negative; every movement is a ledger entry" },
  { value: "EU", label: "API and database hosted in Nuremberg, Germany" },
];

const TechHighlightsSection = () => {
  return (
    <section className="py-24 relative overflow-hidden bg-secondary/30">
      <div className="container mx-auto px-4">
        {/* Technical Highlights */}
        <div className="text-center mb-12">
          <h2 className="text-2xl sm:text-3xl font-display font-bold mb-4">
            Built for <span className="text-gradient-primary">Correctness</span>
          </h2>
          <p className="text-muted-foreground max-w-xl mx-auto">
            Points and awards are money-like. The engine is designed so retries, redeliveries and concurrency never corrupt them.
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

        {/* Guarantees */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-6 max-w-4xl mx-auto">
          {guarantees.map((guarantee) => (
            <div
              key={guarantee.label}
              className="text-center p-6 rounded-2xl bg-gradient-card border border-border/50"
            >
              <div className="text-3xl sm:text-4xl font-display font-bold text-gradient-primary mb-2">
                {guarantee.value}
              </div>
              <div className="text-sm text-muted-foreground">{guarantee.label}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default TechHighlightsSection;
