import { useCallback, useMemo } from 'react';
import { useApi } from '@/hooks/useApi';
import { API_VERSION, toQuery } from '@/lib/api-routes';
import type { CursorPage, ID } from './models/common';
import type {
  NotificationTemplate,
  NotificationTemplateFilters,
  CreateNotificationTemplateData,
  UpdateNotificationTemplateData,
  NotificationPreview,
  NotificationPreviewData,
  NotificationChannelSettings,
  UpdateNotificationChannelsData,
  NotificationRecord,
  NotificationHistoryFilters,
  NotificationStats,
  NotificationStatsParams,
} from './models/notifications';

export const NOTIFICATION_ENDPOINTS = {
  TEMPLATES: `${API_VERSION}/notifications/templates`,
  TEMPLATE: (id: ID) => `${API_VERSION}/notifications/templates/${id}`,
  TEMPLATE_PREVIEW: (id: ID) => `${API_VERSION}/notifications/templates/${id}/preview`,
  CHANNELS: `${API_VERSION}/notifications/channels`,
  HISTORY: `${API_VERSION}/notifications/history`,
  STATS: `${API_VERSION}/notifications/stats`,
} as const;

/** Writes report errors to the caller (forms render them per field), not as toasts. */
const QUIET = { showErrorToast: false } as const;

export function useNotificationsService() {
  const api = useApi();

  const listTemplates = useCallback(
    (filters?: NotificationTemplateFilters) =>
      api.get<CursorPage<NotificationTemplate>>(`${NOTIFICATION_ENDPOINTS.TEMPLATES}${toQuery(filters)}`),
    [api],
  );

  const getTemplate = useCallback(
    (templateId: ID) => api.get<NotificationTemplate>(NOTIFICATION_ENDPOINTS.TEMPLATE(templateId)),
    [api],
  );

  /** 422 notification_template_invalid carries per-field errors; 409 notification_template_name_taken. */
  const createTemplate = useCallback(
    (data: CreateNotificationTemplateData) =>
      api.post<NotificationTemplate>(NOTIFICATION_ENDPOINTS.TEMPLATES, data, QUIET),
    [api],
  );

  const updateTemplate = useCallback(
    (templateId: ID, data: UpdateNotificationTemplateData) =>
      api.patch<NotificationTemplate>(NOTIFICATION_ENDPOINTS.TEMPLATE(templateId), data, QUIET),
    [api],
  );

  /** Soft delete; its history stays. */
  const deleteTemplate = useCallback(
    (templateId: ID) => api.delete<void>(NOTIFICATION_ENDPOINTS.TEMPLATE(templateId), QUIET),
    [api],
  );

  /** Renders the saved template against sample data, or a real player's fields. */
  const previewTemplate = useCallback(
    (templateId: ID, data?: NotificationPreviewData) =>
      api.post<NotificationPreview>(NOTIFICATION_ENDPOINTS.TEMPLATE_PREVIEW(templateId), data ?? {}, QUIET),
    [api],
  );

  const getChannels = useCallback(
    () => api.get<NotificationChannelSettings>(NOTIFICATION_ENDPOINTS.CHANNELS),
    [api],
  );

  const updateChannels = useCallback(
    (data: UpdateNotificationChannelsData) =>
      api.patch<NotificationChannelSettings>(NOTIFICATION_ENDPOINTS.CHANNELS, data, QUIET),
    [api],
  );

  const listHistory = useCallback(
    (filters?: NotificationHistoryFilters) =>
      api.get<CursorPage<NotificationRecord>>(`${NOTIFICATION_ENDPOINTS.HISTORY}${toQuery(filters)}`),
    [api],
  );

  const getStats = useCallback(
    (params?: NotificationStatsParams) =>
      api.get<NotificationStats>(`${NOTIFICATION_ENDPOINTS.STATS}${toQuery(params)}`),
    [api],
  );

  return useMemo(
    () => ({
      listTemplates,
      getTemplate,
      createTemplate,
      updateTemplate,
      deleteTemplate,
      previewTemplate,
      getChannels,
      updateChannels,
      listHistory,
      getStats,
    }),
    [listTemplates, getTemplate, createTemplate, updateTemplate, deleteTemplate, previewTemplate, getChannels, updateChannels, listHistory, getStats],
  );
}
