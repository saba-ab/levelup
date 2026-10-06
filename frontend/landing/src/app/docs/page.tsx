import type { Metadata } from "next";
import { ArrowRight, BookOpen, Code2, FileCode } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import DocsRedirect from "@/components/DocsRedirect";
import { PORTAL_ROUTES } from "@/lib/constants";

export const metadata: Metadata = {
  title: "Documentation - LevelUpOS",
  description: "LevelUpOS documentation and API reference live in the LevelUp portal.",
};

const destinations = [
  {
    icon: BookOpen,
    title: "Documentation",
    description: "Guides to the portal, rules and gamification mechanics.",
    href: PORTAL_ROUTES.DOCS,
  },
  {
    icon: FileCode,
    title: "API Reference",
    description: "The /api/v1 endpoints with their request and response shapes.",
    href: PORTAL_ROUTES.API_REFERENCE,
  },
  {
    icon: Code2,
    title: "Developer Guide",
    description: "How to integrate your backend with the LevelUp API.",
    href: PORTAL_ROUTES.DEVELOPER_DOCS,
  },
];

export default function DocsPage() {
  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <main className="pt-16">
        <section className="relative overflow-hidden bg-gradient-hero min-h-[calc(100vh-4rem)] flex items-center">
          <div className="absolute inset-0 overflow-hidden pointer-events-none">
            <div className="absolute top-1/4 -left-32 w-96 h-96 bg-cyan/10 rounded-full blur-3xl" />
            <div className="absolute bottom-1/4 -right-32 w-96 h-96 bg-purple/10 rounded-full blur-3xl" />
          </div>
          <div className="container mx-auto px-4 py-20 relative z-10">
            <div className="max-w-3xl mx-auto text-center mb-12">
              <h1 className="text-4xl sm:text-5xl font-display font-bold leading-tight mb-6">
                The docs live in the <span className="text-gradient-primary">LevelUp portal</span>
              </h1>
              <p className="text-lg text-muted-foreground mb-4">
                Sign in, or create a free account, to read the guides and the full API reference.
              </p>
              <DocsRedirect href={PORTAL_ROUTES.DOCS} />
            </div>

            <div className="grid md:grid-cols-3 gap-6 max-w-5xl mx-auto">
              {destinations.map((destination) => (
                <a
                  key={destination.title}
                  href={destination.href}
                  className="group bg-gradient-card rounded-2xl border border-border/50 p-6 hover:border-cyan/30 transition-all duration-300 hover:-translate-y-1 card-shadow"
                >
                  <div className="w-12 h-12 rounded-xl bg-cyan/10 flex items-center justify-center mb-4 group-hover:scale-110 transition-transform">
                    <destination.icon className="w-6 h-6 text-cyan" />
                  </div>
                  <h2 className="text-lg font-display font-semibold text-foreground mb-2 flex items-center gap-2">
                    {destination.title}
                    <ArrowRight className="w-4 h-4 opacity-0 group-hover:opacity-100 transition-opacity" />
                  </h2>
                  <p className="text-sm text-muted-foreground leading-relaxed">{destination.description}</p>
                </a>
              ))}
            </div>
          </div>
        </section>
      </main>
      <Footer />
    </div>
  );
}
