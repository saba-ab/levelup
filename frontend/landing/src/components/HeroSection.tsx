import { Button } from "@/components/ui/button";
import { ArrowRight, BookOpen, CheckCircle2, Trophy, Star, Zap, Target } from "lucide-react";
import { PORTAL_ROUTES } from "@/lib/constants";

const HeroSection = () => {
  return (
    <section className="relative min-h-screen flex items-center justify-center overflow-hidden bg-gradient-hero pt-16">
      {/* Background Effects */}
      <div className="absolute inset-0 overflow-hidden">
        {/* Gradient orbs */}
        <div className="absolute top-1/4 -left-32 w-96 h-96 bg-cyan/20 rounded-full blur-3xl animate-pulse-glow" />
        <div className="absolute bottom-1/4 -right-32 w-96 h-96 bg-purple/20 rounded-full blur-3xl animate-pulse-glow animation-delay-300" />
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-gold/5 rounded-full blur-3xl" />

        {/* Grid pattern */}
        <div
          className="absolute inset-0 opacity-[0.03]"
          style={{
            backgroundImage: `linear-gradient(to right, hsl(var(--foreground)) 1px, transparent 1px),
                             linear-gradient(to bottom, hsl(var(--foreground)) 1px, transparent 1px)`,
            backgroundSize: '60px 60px'
          }}
        />
      </div>

      {/* Content */}
      <div className="container mx-auto px-4 relative z-10">
        <div className="grid lg:grid-cols-2 gap-12 items-center">
          {/* Left Column - Text */}
          <div className="text-center lg:text-left">
            {/* Badge */}
            <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-secondary/50 border border-border mb-8 animate-fade-up">
              <Zap className="w-4 h-4 text-gold" />
              <span className="text-sm text-muted-foreground">
                Rules engine with a built-in simulator
              </span>
            </div>

            {/* Headline */}
            <h1 className="text-4xl sm:text-5xl lg:text-6xl xl:text-7xl font-display font-bold leading-tight mb-6 animate-fade-up animation-delay-100">
              Transform User Engagement with{" "}
              <span className="text-gradient-primary">Powerful Gamification</span>
            </h1>

            {/* Subheadline */}
            <p className="text-lg sm:text-xl text-muted-foreground mb-8 max-w-xl mx-auto lg:mx-0 animate-fade-up animation-delay-200">
              One API for points, XP and levels, badges, missions, streaks, rewards and leaderboards. Send what your users do; your rules decide what they earn.
            </p>

            {/* CTAs */}
            <div className="flex flex-col sm:flex-row gap-4 justify-center lg:justify-start animate-fade-up animation-delay-300">
              <Button variant="hero" size="xl" asChild>
                <a href={PORTAL_ROUTES.SIGNUP}>
                  Start Building
                  <ArrowRight className="w-5 h-5" />
                </a>
              </Button>
              <Button variant="heroOutline" size="xl" asChild>
                <a href="/docs">
                  <BookOpen className="w-5 h-5" />
                  Read the Docs
                </a>
              </Button>
            </div>

            {/* Product facts */}
            <ul className="mt-12 flex flex-wrap gap-x-6 gap-y-3 justify-center lg:justify-start text-sm text-muted-foreground animate-fade-up animation-delay-400">
              {["Idempotent by design", "Tenant-isolated data", "API hosted in Germany", "TypeScript & Python SDKs"].map((fact) => (
                <li key={fact} className="flex items-center gap-2">
                  <CheckCircle2 className="w-4 h-4 text-cyan" />
                  <span>{fact}</span>
                </li>
              ))}
            </ul>
          </div>

          {/* Right Column - Visual */}
          <div className="relative hidden lg:block">
            {/* Main Dashboard Card */}
            <div className="relative bg-gradient-card rounded-2xl border border-border/50 p-6 card-shadow animate-float">
              {/* Header */}
              <div className="flex items-center justify-between mb-6">
                <h3 className="font-display font-semibold text-foreground">User Dashboard</h3>
                <div className="flex items-center gap-2 text-gold">
                  <Star className="w-4 h-4 fill-current" />
                  <span className="text-sm font-medium">Level 24</span>
                </div>
              </div>

              {/* XP Progress */}
              <div className="mb-6">
                <div className="flex justify-between text-sm mb-2">
                  <span className="text-muted-foreground">Experience Points</span>
                  <span className="text-foreground font-medium">8,450 / 10,000 XP</span>
                </div>
                <div className="h-3 bg-secondary rounded-full overflow-hidden">
                  <div className="h-full w-[84%] bg-gradient-to-r from-cyan to-purple rounded-full relative">
                    <div className="absolute inset-0 bg-white/20 animate-pulse-glow" />
                  </div>
                </div>
              </div>

              {/* Stats Grid */}
              <div className="grid grid-cols-3 gap-4 mb-6">
                {[
                  { icon: Trophy, label: "Badges", value: "23", color: "text-gold" },
                  { icon: Target, label: "Missions", value: "47", color: "text-cyan" },
                  { icon: Zap, label: "Streak", value: "12 days", color: "text-purple" },
                ].map((stat) => (
                  <div key={stat.label} className="bg-secondary/50 rounded-xl p-3 text-center">
                    <stat.icon className={`w-5 h-5 mx-auto mb-1 ${stat.color}`} />
                    <div className="text-lg font-bold text-foreground">{stat.value}</div>
                    <div className="text-xs text-muted-foreground">{stat.label}</div>
                  </div>
                ))}
              </div>

              {/* Recent Badges */}
              <div>
                <div className="text-sm text-muted-foreground mb-3">Recent Achievements</div>
                <div className="flex gap-2">
                  {["🏆", "⭐", "🎯", "🔥", "💎"].map((emoji, i) => (
                    <div
                      key={i}
                      className="w-10 h-10 rounded-lg bg-secondary/80 flex items-center justify-center text-lg hover:scale-110 transition-transform cursor-pointer"
                    >
                      {emoji}
                    </div>
                  ))}
                </div>
              </div>
            </div>

            {/* Floating Elements */}
            <div className="absolute -top-4 -right-4 bg-card rounded-xl border border-border p-4 card-shadow animate-float animation-delay-200">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-gold/20 flex items-center justify-center">
                  <Trophy className="w-5 h-5 text-gold" />
                </div>
                <div>
                  <div className="text-sm font-semibold text-foreground">Badge Earned!</div>
                  <div className="text-xs text-muted-foreground">Gold Achievement</div>
                </div>
              </div>
            </div>

            <div className="absolute -bottom-4 -left-4 bg-card rounded-xl border border-border p-4 card-shadow animate-float animation-delay-400">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-cyan/20 flex items-center justify-center">
                  <Zap className="w-5 h-5 text-cyan" />
                </div>
                <div>
                  <div className="text-sm font-semibold text-foreground">+250 XP</div>
                  <div className="text-xs text-muted-foreground">Mission Complete</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default HeroSection;
