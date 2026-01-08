# URL Configuration Summary

This document outlines all the external URLs configured in the LevelUpOS landing page.

## Domain Structure

- **Landing Page**: `www.levelupos.ge` (production)
- **Portal/Dashboard**: `portal.levelupos.com`
- **Documentation**: `docs.levelupos.com`
- **Status Page**: `status.levelupos.com`

## URL Constants (`src/lib/constants.ts`)

```typescript
export const PORTAL_URL = 'https://portal.levelupos.com';
export const DOCS_URL = 'https://docs.levelupos.com';

export const PORTAL_ROUTES = {
  LOGIN: `${PORTAL_URL}/login`,
  SIGNUP: `${PORTAL_URL}/signup`,
  DASHBOARD: `${PORTAL_URL}/dashboard`,
  DOCS: DOCS_URL,
}
```

## Button/Link Mapping

### Navbar
- **"Sign In"** → `portal.levelupos.com/login`
- **"Get Started Free"** → `portal.levelupos.com/signup`
- **Logo** → `/` (home page)

### Hero Section
- **"Start Building"** → `portal.levelupos.com/signup`
- **"See API Demo"** → `docs.levelupos.com`

### Pricing Section
- **"Get Started Free"** (Free & Pro tiers) → `portal.levelupos.com/signup`
- **"Start Pro Trial"** → `portal.levelupos.com/signup`
- **"Contact Sales"** (Enterprise) → `mailto:sales@levelupos.com`

### API Preview Section
- **"View Full Docs"** → `docs.levelupos.com`
- **"Try in Playground"** → `portal.levelupos.com/dashboard/playground`

### CTA Section
- **"Get Started Free"** → `portal.levelupos.com/signup`
- **"Schedule Demo"** → `mailto:sales@levelupos.com`

### Footer

#### Product
- Features → `/#features`
- Pricing → `/#pricing`
- Use Cases → `/#use-cases`
- Changelog → `portal.levelupos.com/dashboard/changelog`

#### Developers
- Documentation → `docs.levelupos.com`
- API Reference → `docs.levelupos.com/api`
- SDKs → `docs.levelupos.com/sdks`
- Status → `status.levelupos.com`

#### Company
- About → `/about`
- Blog → `/blog`
- Careers → `/careers`
- Contact → `mailto:hello@levelupos.com`

#### Legal
- Privacy → `/privacy`
- Terms → `/terms`
- Security → `/security`

#### Social Media
- Twitter → `https://twitter.com/levelupos`
- GitHub → `https://github.com/levelupos`
- LinkedIn → `https://linkedin.com/company/levelupos`
- YouTube → `https://youtube.com/@levelupos`

## Notes

1. All portal login/signup actions route to `portal.levelupos.com`
2. Documentation links point to `docs.levelupos.com`
3. Internal links (Features, Pricing, etc.) use hash anchors (`/#features`)
4. External links include `target="_blank"` and `rel="noopener noreferrer"` for security
5. Contact/demo requests use `mailto:` links

## Future Pages to Create (in Next.js)

These routes are referenced but need to be created:
- `/about`
- `/blog`
- `/careers`
- `/privacy`
- `/terms`
- `/security`
