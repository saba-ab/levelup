// React Query hooks for data fetching with caching and optimistic updates

// Query keys factory
export { queryKeys } from './keys';

// Player queries and mutations
export {
  usePlayersQuery,
  usePlayerQuery,
  usePlayerBadgesQuery,
  usePlayerMissionsQuery,
  usePlayerStreaksQuery as usePlayersStreaksQuery,
  usePlayerLevelQuery,
  useCreatePlayerMutation,
  useUpdatePlayerMutation,
  useDeletePlayerMutation,
  useAwardBadgeMutation,
  useRevokeBadgeMutation,
  useGrantXpMutation,
  // Wallets
  usePlayerWalletQuery,
  useWalletTransactionsQuery,
  useCreditWalletMutation,
  useDebitWalletMutation,
  useTransferPointsMutation,
} from './players';

// Mechanics queries and mutations
export {
  // Badges
  useBadgesQuery,
  useBadgeQuery,
  usePlayerBadgesQuery as useMechanicsPlayerBadgesQuery,
  useCreateBadgeMutation,
  useUpdateBadgeMutation,
  useDeleteBadgeMutation,
  // Levels
  useLevelsQuery,
  useLevelQuery,
  useCreateLevelMutation,
  useUpdateLevelMutation,
  useDeleteLevelMutation,
  // Missions
  useMissionsQuery,
  useMissionQuery,
  usePlayerMissionsQuery as useMechanicsPlayerMissionsQuery,
  useCreateMissionMutation,
  useUpdateMissionMutation,
  useDeleteMissionMutation,
  useStartMissionMutation,
  useUpdateMissionProgressMutation,
  useCompleteMissionMutation,
  // Streaks
  useStreaksQuery,
  useStreakQuery,
  usePlayerStreakQuery,
  usePlayerStreaksQuery,
  useCreateStreakMutation,
  useUpdateStreakMutation,
  useDeleteStreakMutation,
  useRecordStreakActivityMutation,
  useResetStreakMutation,
  // Leaderboards
  useLeaderboardsQuery,
  useLeaderboardQuery,
  useLeaderboardEntriesQuery,
  usePlayerRankQuery,
  useCreateLeaderboardMutation,
  useUpdateLeaderboardMutation,
  useDeleteLeaderboardMutation,
  // Rewards
  useRewardsQuery,
  useRewardQuery,
  usePlayerRewardsQuery,
  useCreateRewardMutation,
  useUpdateRewardMutation,
  useDeleteRewardMutation,
  useClaimRewardMutation,
  useRedeemRewardMutation,
} from './mechanics';

// Rules (Go rules module). POST /rules/execute is gone: use simulate or
// POST /activities; decisions replace the Laravel execution log.
export {
  ruleKeys,
  ApiRequestError,
  useRulesQuery,
  useRuleQuery,
  useRuleVersionsQuery,
  useCreateRuleMutation,
  useUpdateRuleMutation,
  useDeleteRuleMutation,
  useCreateRuleVersionMutation,
  usePublishRuleMutation,
  useSimulateRulesMutation,
  useRuleDecisionsQuery,
  useRuleDecisionQuery,
} from './rules';

// Event types (trigger catalogue) and activities
export {
  eventKeys,
  useEventsQuery,
  useEventCategoriesQuery,
  useEventQuery,
  useCreateEventMutation,
  useUpdateEventMutation,
  useDeleteEventMutation,
} from './events';

export {
  activityKeys,
  useActivitiesQuery,
  useActivityQuery,
  useIngestActivityMutation,
} from './activities';

// Users queries and mutations
export {
  useUsersQuery,
  useUserQuery,
  useCreateUserMutation,
  useUpdateUserMutation,
  useAssignUserRolesMutation,
  useDeleteUserMutation,
} from './users';

// Programs queries and mutations (segments do not exist in the Go API)
export {
  // Programs
  useProgramsQuery,
  useProgramQuery,
  useProgramStatsQuery,
  useProgramPlayersQuery,
  useCreateProgramMutation,
  useUpdateProgramMutation,
  useDeleteProgramMutation,
  useActivateProgramMutation,
  usePauseProgramMutation,
  useEndProgramMutation,
  useDuplicateProgramMutation,
} from './programs';
