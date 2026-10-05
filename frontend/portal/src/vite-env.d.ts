/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL_PRODUCTION: string;
  readonly VITE_API_URL_STAGING: string;
  readonly VITE_API_URL_DEVELOPMENT: string;
  readonly VITE_API_URL_LOCALHOST: string;
  readonly VITE_API_URL_DEFAULT: string;
  readonly VITE_APP_NAME: string;
  readonly VITE_APP_ENV: string;
  readonly VITE_DEFAULT_ENVIRONMENT: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
