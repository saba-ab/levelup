import type { ReactNode } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";

export interface ContentSection {
  id: string;
  title: string;
  body: ReactNode;
}

interface ContentPageProps {
  eyebrow: string;
  title: ReactNode;
  intro: ReactNode;
  lastUpdated?: string;
  sections: ContentSection[];
  children?: ReactNode;
}

/** Shared layout for the About and legal pages: same chrome and type scale as the landing page. */
const ContentPage = ({ eyebrow, title, intro, lastUpdated, sections, children }: ContentPageProps) => {
  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <main className="pt-16">
        <section className="relative overflow-hidden bg-gradient-hero">
          <div className="absolute inset-0 overflow-hidden pointer-events-none">
            <div className="absolute -top-24 -left-32 w-96 h-96 bg-cyan/10 rounded-full blur-3xl" />
            <div className="absolute -bottom-24 -right-32 w-96 h-96 bg-purple/10 rounded-full blur-3xl" />
          </div>
          <div className="container mx-auto px-4 py-20 relative z-10">
            <div className="max-w-3xl">
              <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-cyan/10 border border-cyan/20 mb-6">
                <span className="text-sm text-cyan font-medium">{eyebrow}</span>
              </div>
              <h1 className="text-4xl sm:text-5xl font-display font-bold leading-tight mb-6">{title}</h1>
              <div className="text-lg text-muted-foreground leading-relaxed space-y-4">{intro}</div>
              {lastUpdated && (
                <p className="mt-6 text-sm text-muted-foreground">Last updated: {lastUpdated}</p>
              )}
            </div>
          </div>
        </section>

        <section className="py-16">
          <div className="container mx-auto px-4">
            <div className="grid lg:grid-cols-[220px_1fr] gap-12 max-w-6xl">
              <nav aria-label="On this page" className="hidden lg:block">
                <div className="sticky top-24">
                  <p className="text-xs uppercase tracking-wider text-muted-foreground mb-4">On this page</p>
                  <ul className="space-y-2">
                    {sections.map((section) => (
                      <li key={section.id}>
                        <a
                          href={`#${section.id}`}
                          className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                        >
                          {section.title}
                        </a>
                      </li>
                    ))}
                  </ul>
                </div>
              </nav>

              <div className="max-w-3xl space-y-12">
                {sections.map((section) => (
                  <article key={section.id} id={section.id} className="scroll-mt-24">
                    <h2 className="text-2xl font-display font-semibold text-foreground mb-4">{section.title}</h2>
                    <div className="space-y-4 text-muted-foreground leading-relaxed">{section.body}</div>
                  </article>
                ))}
                {children}
              </div>
            </div>
          </div>
        </section>
      </main>
      <Footer />
    </div>
  );
};

export const BulletList = ({ items }: { items: ReactNode[] }) => (
  <ul className="space-y-2 pl-5 list-disc marker:text-cyan">
    {items.map((item, index) => (
      <li key={index}>{item}</li>
    ))}
  </ul>
);

export const Strong = ({ children }: { children: ReactNode }) => (
  <strong className="text-foreground font-semibold">{children}</strong>
);

export const Placeholder = ({ children }: { children: ReactNode }) => (
  <span className="rounded bg-gold/10 px-1.5 py-0.5 text-gold font-medium">{children}</span>
);

export const TextLink = ({ href, children }: { href: string; children: ReactNode }) => (
  <a href={href} className="text-cyan hover:underline">
    {children}
  </a>
);

export default ContentPage;
