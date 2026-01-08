import { Button } from "@/components/ui/button";
import { ArrowRight, Calendar, Sparkles } from "lucide-react";

const CTASection = () => {
  return (
    <section className="py-24 relative overflow-hidden">
      {/* Background Effects */}
      <div className="absolute inset-0">
        <div className="absolute top-0 left-1/4 w-96 h-96 bg-cyan/10 rounded-full blur-3xl" />
        <div className="absolute bottom-0 right-1/4 w-96 h-96 bg-purple/10 rounded-full blur-3xl" />
      </div>

      <div className="container mx-auto px-4 relative z-10">
        <div className="max-w-4xl mx-auto text-center">
          {/* Badge */}
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-gold/10 border border-gold/20 mb-8">
            <Sparkles className="w-4 h-4 text-gold" />
            <span className="text-sm text-gold font-medium">Start Free Today</span>
          </div>

          {/* Headline */}
          <h2 className="text-3xl sm:text-4xl lg:text-5xl font-display font-bold mb-6">
            Ready to{" "}
            <span className="text-gradient-primary">Gamify Your Platform?</span>
          </h2>

          <p className="text-lg text-muted-foreground mb-10 max-w-2xl mx-auto">
            Join 500+ organizations using LevelUpOS to drive engagement, retention, and growth. Get started in minutes with our free tier.
          </p>

          {/* CTAs */}
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Button variant="hero" size="xl">
              Get Started Free
              <ArrowRight className="w-5 h-5" />
            </Button>
            <Button variant="heroOutline" size="xl">
              <Calendar className="w-5 h-5" />
              Schedule Demo
            </Button>
          </div>

          {/* Trust Note */}
          <p className="mt-8 text-sm text-muted-foreground">
            No credit card required • Free tier available • Setup in minutes
          </p>
        </div>
      </div>
    </section>
  );
};

export default CTASection;
