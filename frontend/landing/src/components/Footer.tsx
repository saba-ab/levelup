import { Gamepad2 } from "lucide-react";
import { CONTACT_EMAIL, PORTAL_ROUTES } from "@/lib/constants";

const footerLinks = {
  Product: [
    { name: "Features", href: "/#features" },
    { name: "Pricing", href: "/#pricing" },
    { name: "Use Cases", href: "/#use-cases" },
    { name: "SDKs", href: "/#sdks" },
  ],
  Developers: [
    { name: "Documentation", href: "/docs" },
    { name: "API Reference", href: PORTAL_ROUTES.API_REFERENCE },
    { name: "Developer Guide", href: PORTAL_ROUTES.DEVELOPER_DOCS },
  ],
  Company: [
    { name: "About", href: "/about" },
    { name: "Contact", href: `mailto:${CONTACT_EMAIL}` },
  ],
  Legal: [
    { name: "Privacy", href: "/privacy" },
    { name: "Terms", href: "/terms" },
    { name: "Security", href: "/security" },
  ],
};

const Footer = () => {
  return (
    <footer className="bg-card border-t border-border">
      <div className="container mx-auto px-4 py-16">
        <div className="grid grid-cols-2 md:grid-cols-5 gap-8">
          {/* Brand */}
          <div className="col-span-2 md:col-span-1">
            <a href="/" className="flex items-center gap-2 mb-4">
              <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-cyan to-purple flex items-center justify-center">
                <Gamepad2 className="w-5 h-5 text-primary-foreground" />
              </div>
              <span className="text-xl font-display font-bold text-foreground">
                LevelUp<span className="text-gradient-primary">OS</span>
              </span>
            </a>
            <p className="text-sm text-muted-foreground">
              A multi-tenant gamification API: points, XP, badges, missions, streaks, rewards and leaderboards.
            </p>
          </div>

          {/* Links */}
          {Object.entries(footerLinks).map(([category, links]) => (
            <div key={category}>
              <h4 className="font-display font-semibold text-foreground mb-4">
                {category}
              </h4>
              <ul className="space-y-3">
                {links.map((link) => (
                  <li key={link.name}>
                    <a
                      href={link.href}
                      className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {link.name}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {/* Bottom Bar */}
        <div className="mt-16 pt-8 border-t border-border flex flex-col sm:flex-row justify-between items-center gap-4">
          <p className="text-sm text-muted-foreground">
            © {new Date().getFullYear()} LevelUpOS. All rights reserved.
          </p>
          <p className="text-sm text-muted-foreground">
            Made with ❤️ for developers
          </p>
        </div>
      </div>
    </footer>
  );
};

export default Footer;
