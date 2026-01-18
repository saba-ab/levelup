# URL Configuration Summary

This document outlines all the external URLs configured in the LevelUpOS landing page.

## Domain Structure

- **Landing Page**: `levelupos.ge` (production)
- **Portal/Dashboard/Docs**: `portal.levelupos.ge` (all portal resources)
- **Status Page**: `status.levelupos.ge`

## URL Constants (`src/lib/constants.ts`)

```typescript
// External URLs - All portal resources are on portal.levelupos.ge
export const PORTAL_URL = 'https://portal.levelupos.ge';

// Portal routes
export const PORTAL_ROUTES = {
  LOGIN: `${PORTAL_URL}/login`,
  SIGNUP: `${PORTAL_URL}/signup`,
  DASHBOARD: `${PORTAL_URL}/dashboard`,
  DOCS: `${PORTAL_URL}/docs`,
  PLAYGROUND: `${PORTAL_URL}/playground`,
}
```

## Button/Link Mapping

### Navbar
- **"Sign In"** → `portal.levelupos.ge/login`
- **"Get Started Free"** → `portal.levelupos.ge/signup`
- **Logo** → `/` (home page)

### Hero Section
- **"Start Building"** → `portal.levelupos.ge/signup`
- **"See API Demo"** → `portal.levelupos.ge/docs`

### Pricing Section
- **"Get Started Free"** (Free & Pro tiers) → `portal.levelupos.ge/signup`
- **"Start Pro Trial"** → `portal.levelupos.ge/signup`
- **"Contact Sales"** (Enterprise) → `mailto:sales@levelupos.com`

### API Preview Section
- **"View Full Docs"** → `portal.levelupos.ge/docs`
- **"Try in Playground"** → `portal.levelupos.ge/playground`

### CTA Section
- **"Get Started Free"** → `portal.levelupos.ge/signup`
- **"Schedule Demo"** → `mailto:sales@levelupos.com`

### Footer

#### Product
- Features → `/#features`
- Pricing → `/#pricing`
- Use Cases → `/#use-cases`
- Changelog → `portal.levelupos.ge/dashboard/changelog`

#### Developers
- Documentation → `portal.levelupos.ge/docs`
- API Reference → `portal.levelupos.ge/docs/api`
- SDKs → `portal.levelupos.ge/docs/sdks`
- Status → `status.levelupos.ge`

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

1. **All portal resources** (login, signup, dashboard, docs) are on `portal.levelupos.ge`
2. **Landing page only** is on `levelupos.ge`
3. Internal navigation links (Features, Pricing, etc.) use hash anchors (`/#features`)
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
