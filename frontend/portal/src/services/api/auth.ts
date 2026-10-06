import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import { AuthResponse, RegisterData, LoginData, MeResponse, Tenant, UpdateTenantData } from './types';
import { API_VERSION, AUTH_ENDPOINTS, TENANT_ENDPOINTS } from '@/lib/api-routes';
import { authStorage } from '@/lib/auth-storage';
import type {
  AcceptInviteData,
  ForgotPasswordData,
  InvitationPreview,
  ResetPasswordData,
  VerifyEmailData,
} from './models/identity';

/** Self-service account flows (identity module). All but resend are anonymous. */
export const ACCOUNT_ENDPOINTS = {
  FORGOT_PASSWORD: `${API_VERSION}/auth/forgot-password`,
  RESET_PASSWORD: `${API_VERSION}/auth/reset-password`,
  VERIFY_EMAIL: `${API_VERSION}/auth/verify-email`,
  RESEND_VERIFICATION: `${API_VERSION}/auth/resend-verification`,
  INVITATION_PREVIEW: (token: string) => `${API_VERSION}/auth/invitations/${encodeURIComponent(token)}`,
  ACCEPT_INVITE: `${API_VERSION}/auth/accept-invite`,
} as const;

/** Errors are rendered by the calling page, not as toasts. */
const QUIET = { showErrorToast: false } as const;
const ANON_QUIET = { skipAuth: true, showErrorToast: false } as const;

export function useAuthService() {
  const api = useApi();

  /** Creates a tenant and its owner; returns a session. */
  const register = useCallback(
    (data: RegisterData) => api.post<AuthResponse>(AUTH_ENDPOINTS.REGISTER, data, { skipAuth: true }),
    [api],
  );

  const login = useCallback(
    (data: LoginData) => api.post<AuthResponse>(AUTH_ENDPOINTS.LOGIN, data, { skipAuth: true }),
    [api],
  );

  const me = useCallback(() => api.get<MeResponse>(AUTH_ENDPOINTS.ME, QUIET), [api]);

  /** Rotates the stored refresh token (single-use) for a new session. */
  const refresh = useCallback(
    () => api.post<AuthResponse>(AUTH_ENDPOINTS.REFRESH, { refresh_token: authStorage.getRefreshToken() }, { skipAuth: true }),
    [api],
  );

  /** Denies the current access token and revokes every refresh token. */
  const logout = useCallback(() => api.post<void>(AUTH_ENDPOINTS.LOGOUT), [api]);

  const getTenant = useCallback(() => api.get<Tenant>(TENANT_ENDPOINTS.CURRENT), [api]);
  const updateTenant = useCallback((data: UpdateTenantData) => api.patch<Tenant>(TENANT_ENDPOINTS.CURRENT, data), [api]);

  /** Always 202 (no account enumeration); at most 3 emails per address per hour. */
  const forgotPassword = useCallback(
    (data: ForgotPasswordData) => api.post<void>(ACCOUNT_ENDPOINTS.FORGOT_PASSWORD, data, ANON_QUIET),
    [api],
  );

  /** 204 on success; 422 invalid_reset_token for a bad, used or expired token. */
  const resetPassword = useCallback(
    (data: ResetPasswordData) => api.post<void>(ACCOUNT_ENDPOINTS.RESET_PASSWORD, data, ANON_QUIET),
    [api],
  );

  /** 204 on success; 422 invalid_verification_token otherwise. */
  const verifyEmail = useCallback(
    (data: VerifyEmailData) => api.post<void>(ACCOUNT_ENDPOINTS.VERIFY_EMAIL, data, ANON_QUIET),
    [api],
  );

  /** 202; 409 email_already_verified. Earlier links stop working. */
  const resendVerification = useCallback(
    () => api.post<void>(ACCOUNT_ENDPOINTS.RESEND_VERIFICATION, undefined, QUIET),
    [api],
  );

  /** 404 invitation_not_found unless pending in an active tenant. */
  const previewInvitation = useCallback(
    (token: string) => api.get<InvitationPreview>(ACCOUNT_ENDPOINTS.INVITATION_PREVIEW(token), ANON_QUIET),
    [api],
  );

  /** 201 with a session, like login. 409 email_taken, 422 invalid_invitation_token. */
  const acceptInvite = useCallback(
    (data: AcceptInviteData) => api.post<AuthResponse>(ACCOUNT_ENDPOINTS.ACCEPT_INVITE, data, ANON_QUIET),
    [api],
  );

  return useMemo(
    () => ({
      register,
      login,
      me,
      refresh,
      logout,
      getTenant,
      updateTenant,
      forgotPassword,
      resetPassword,
      verifyEmail,
      resendVerification,
      previewInvitation,
      acceptInvite,
    }),
    [
      register,
      login,
      me,
      refresh,
      logout,
      getTenant,
      updateTenant,
      forgotPassword,
      resetPassword,
      verifyEmail,
      resendVerification,
      previewInvitation,
      acceptInvite,
    ],
  );
}
