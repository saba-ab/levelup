// Where the landing page links to. Override per environment:
//   NEXT_PUBLIC_PORTAL_URL  (dev: http://localhost:5173)
//   NEXT_PUBLIC_API_URL     (dev: http://localhost:8080), shown in the API examples
export const PORTAL_URL = process.env.NEXT_PUBLIC_PORTAL_URL || 'https://portal.levelupos.ge';
export const API_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.levelupos.ge';

// Portal routes (frontend/portal/src/App.tsx). Every entry must exist there.
export const PORTAL_ROUTES = {
  HOME: `${PORTAL_URL}/`,
  LOGIN: `${PORTAL_URL}/login`,
  SIGNUP: `${PORTAL_URL}/register`,
  DOCS: `${PORTAL_URL}/docs`,
  API_REFERENCE: `${PORTAL_URL}/docs/api`,
  DEVELOPER_DOCS: `${PORTAL_URL}/docs/developer`,
  /** Rules → Simulate: evaluate an activity against live rules without side effects. */
  SIMULATOR: `${PORTAL_URL}/rules`,
} as const;

export const CONTACT_EMAIL = 'hello@levelupos.com';
export const SALES_EMAIL = 'sales@levelupos.com';

// Operator details shown on the legal pages. Every value in brackets is a
// placeholder the site owner must replace before launch.
export const LEGAL = {
  COMPANY_NAME: '[Company legal name]',
  COMPANY_ADDRESS: '[Registered address]',
  COMPANY_REGISTRATION: '[Company registration number]',
  PRIVACY_CONTACT: '[Privacy contact email]',
  SECURITY_CONTACT: '[Security contact email]',
  EMAIL_PROVIDER: '[Transactional email provider]',
  GOVERNING_LAW: '[Governing law and jurisdiction]',
  SUPERVISORY_AUTHORITY: '[Competent data protection authority]',
  BACKUP_RETENTION: '[Backup retention period]',
  LAST_UPDATED: '6 October 2026',
} as const;

// Official SDK package names (sdks/typescript, sdks/python).
export const SDK_PACKAGES = {
  TYPESCRIPT: '@levelup/sdk',
  PYTHON: 'levelup',
} as const;
