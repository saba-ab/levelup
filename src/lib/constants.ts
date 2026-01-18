// External URLs - All portal resources are on portal.levelupos.ge
export const PORTAL_URL = 'https://portal.levelupos.ge';

// Portal routes
export const PORTAL_ROUTES = {
  HOME: `${PORTAL_URL}`,
  LOGIN: `${PORTAL_URL}/login`,
  SIGNUP: `${PORTAL_URL}/signup`,
  DASHBOARD: `${PORTAL_URL}/dashboard`,
  DOCS: `${PORTAL_URL}/docs`,
  PLAYGROUND: `${PORTAL_URL}/playground`,
} as const;
