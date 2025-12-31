// React Query hooks for data fetching with caching and optimistic updates

// Query keys factory
export { queryKeys } from './keys';

// Player queries and mutations
export {
  usePlayersQuery,
  usePlayerQuery,
  usePlayerBadgesQuery,
  usePlayerMissionsQuery,
  usePlayerStreaksQuery,
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
