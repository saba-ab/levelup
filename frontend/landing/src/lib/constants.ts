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
