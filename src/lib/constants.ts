// External URLs
export const PORTAL_URL = 'https://portal.levelupos.com';
export const DOCS_URL = 'https://docs.levelupos.com';

// Portal routes
export const PORTAL_ROUTES = {
  LOGIN: `${PORTAL_URL}/login`,
  SIGNUP: `${PORTAL_URL}/signup`,
  DASHBOARD: `${PORTAL_URL}/dashboard`,
  DOCS: DOCS_URL,
} as const;
