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
  // Wallets
  usePlayerWalletQuery,
  useWalletTransactionsQuery,
  useCreditWalletMutation,
  useDebitWalletMutation,
  useTransferPointsMutation,
  // Rules
  useRulesQuery,
  useRuleQuery,
  useRuleExecutionsQuery,
  useCreateRuleMutation,
  useUpdateRuleMutation,
  useDeleteRuleMutation,
  useCreateRuleVersionMutation,
  useExecuteRulesMutation,
} from './mechanics';

// Users queries and mutations
export {
  useUsersQuery,
  useUserQuery,
  useCreateUserMutation,
  useUpdateUserMutation,
  useDeleteUserMutation,
} from './users';

// Programs and Segments queries and mutations
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
  // Segments
  useSegmentsQuery,
  useSegmentQuery,
  useSegmentPlayersQuery,
  useCreateSegmentMutation,
  useUpdateSegmentMutation,
  useDeleteSegmentMutation,
  useRefreshDynamicSegmentMutation,
  usePreviewSegmentRulesMutation,
} from './programs';
