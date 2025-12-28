// React Query hooks for data fetching with caching and optimistic updates

// Query keys factory
export { queryKeys } from './keys';

// Player queries and mutations
export {
  usePlayersQuery,
  usePlayerQuery,
  usePlayerStatsQuery,
  usePlayerBadgesQuery,
  usePlayerMissionsQuery,
  usePlayerStreaksQuery,
  useCreatePlayerMutation,
  useUpdatePlayerMutation,
  useDeletePlayerMutation,
  useAwardBadgeMutation,
  useRevokeBadgeMutation,
  useAwardPointsMutation,
  useDeductPointsMutation,
} from './players';

// Mechanics queries and mutations
export {
  // Badges
  useBadgesQuery,
  useBadgeQuery,
  useCreateBadgeMutation,
  useUpdateBadgeMutation,
  useDeleteBadgeMutation,
  // Levels
  useLevelsQuery,
  useLevelQuery,
  useCreateLevelMutation,
  useUpdateLevelMutation,
  useDeleteLevelMutation,
  useReorderLevelsMutation,
  // Missions
  useMissionsQuery,
  useMissionQuery,
  useCreateMissionMutation,
  useUpdateMissionMutation,
  useDeleteMissionMutation,
  // Streaks
  useStreaksQuery,
  useCreateStreakMutation,
  useUpdateStreakMutation,
  useDeleteStreakMutation,
  // Leaderboards
  useLeaderboardsQuery,
  useLeaderboardEntriesQuery,
  useCreateLeaderboardMutation,
  useDeleteLeaderboardMutation,
  // Rewards
  useRewardsQuery,
  useRewardQuery,
  useCreateRewardMutation,
  useUpdateRewardMutation,
  useDeleteRewardMutation,
  useRedeemRewardMutation,
  // Point Wallets
  usePointWalletsQuery,
  useCreatePointWalletMutation,
  useDeletePointWalletMutation,
} from './mechanics';

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