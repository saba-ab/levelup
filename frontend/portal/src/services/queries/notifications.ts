import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useNotificationsService } from '@/services/api/notifications';
import type { ID } from '@/services/api/models/common';
import type {
  NotificationTemplateFilters,
  CreateNotificationTemplateData,
  UpdateNotificationTemplateData,
  NotificationPreviewData,
  UpdateNotificationChannelsData,
  NotificationHistoryFilters,
  NotificationStatsParams,
} from '@/services/api/models/notifications';
import { unwrap } from './rules';

export const notificationKeys = {
  all: ['notifications'] as const,
  templates: () => [...notificationKeys.all, 'templates'] as const,
  templateList: (filters?: NotificationTemplateFilters) => [...notificationKeys.templates(), 'list', filters] as const,
  template: (id: ID) => [...notificationKeys.templates(), 'detail', id] as const,
  channels: () => [...notificationKeys.all, 'channels'] as const,
  history: (filters?: NotificationHistoryFilters) => [...notificationKeys.all, 'history', filters] as const,
  stats: (params?: NotificationStatsParams) => [...notificationKeys.all, 'stats', params] as const,
};

// ---- templates ------------------------------------------------------------------

/** One cursor page of templates: { data, next_cursor }. */
export function useNotificationTemplatesQuery(filters?: NotificationTemplateFilters, enabled = true) {
  const { listTemplates } = useNotificationsService();
  return useQuery({
    queryKey: notificationKeys.templateList(filters),
    queryFn: async () => unwrap(await listTemplates(filters), 'Failed to fetch notification templates'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

/** 422 notification_template_invalid (field errors), 409 notification_template_name_taken. */
export function useCreateNotificationTemplateMutation() {
  const queryClient = useQueryClient();
  const { createTemplate } = useNotificationsService();
  return useMutation({
    mutationFn: async (data: CreateNotificationTemplateData) =>
      unwrap(await createTemplate(data), 'Failed to create notification template'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: notificationKeys.templates() }),
  });
}

export function useUpdateNotificationTemplateMutation() {
  const queryClient = useQueryClient();
  const { updateTemplate } = useNotificationsService();
  return useMutation({
    mutationFn: async ({ templateId, data }: { templateId: ID; data: UpdateNotificationTemplateData }) =>
      unwrap(await updateTemplate(templateId, data), 'Failed to update notification template'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: notificationKeys.templates() }),
  });
}

export function useDeleteNotificationTemplateMutation() {
  const queryClient = useQueryClient();
  const { deleteTemplate } = useNotificationsService();
  return useMutation({
    mutationFn: async (templateId: ID) => {
      unwrap(await deleteTemplate(templateId), 'Failed to delete notification template');
      return templateId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: notificationKeys.templates() }),
  });
}

/** Renders a saved template (sample data, or the chosen player's fields). */
export function usePreviewNotificationTemplateMutation() {
  const { previewTemplate } = useNotificationsService();
  return useMutation({
    mutationFn: async ({ templateId, data }: { templateId: ID; data?: NotificationPreviewData }) =>
      unwrap(await previewTemplate(templateId, data), 'Failed to preview notification template'),
  });
}

// ---- channels ---------------------------------------------------------------------

export function useNotificationChannelsQuery(enabled = true) {
  const { getChannels } = useNotificationsService();
  return useQuery({
    queryKey: notificationKeys.channels(),
    queryFn: async () => unwrap(await getChannels(), 'Failed to fetch notification channels'),
    enabled,
  });
}

export function useUpdateNotificationChannelsMutation() {
  const queryClient = useQueryClient();
  const { updateChannels } = useNotificationsService();
  return useMutation({
    mutationFn: async (data: UpdateNotificationChannelsData) =>
      unwrap(await updateChannels(data), 'Failed to update notification channels'),
    onSuccess: (settings) => queryClient.setQueryData(notificationKeys.channels(), settings),
  });
}

// ---- history & stats ----------------------------------------------------------------

/** One cursor page of delivery history, newest first. */
export function useNotificationHistoryQuery(filters?: NotificationHistoryFilters, enabled = true) {
  const { listHistory } = useNotificationsService();
  return useQuery({
    queryKey: notificationKeys.history(filters),
    queryFn: async () => unwrap(await listHistory(filters), 'Failed to fetch notification history'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function useNotificationStatsQuery(params?: NotificationStatsParams, enabled = true) {
  const { getStats } = useNotificationsService();
  return useQuery({
    queryKey: notificationKeys.stats(params),
    queryFn: async () => unwrap(await getStats(params), 'Failed to fetch notification stats'),
    placeholderData: keepPreviousData,
    enabled,
  });
}
