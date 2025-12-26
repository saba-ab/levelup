// Sample data for the LevelUpOs dashboard

export const programs = [
  { id: '1', name: 'Onboarding XP', status: 'active', eventsCount: 12450, createdAt: '2024-01-15' },
  { id: '2', name: 'Loyalty Rewards', status: 'active', eventsCount: 45230, createdAt: '2024-02-01' },
  { id: '3', name: 'Weekly Challenges', status: 'draft', eventsCount: 8920, createdAt: '2024-03-10' },
];

export const rules = [
  { id: '1', name: 'First Purchase Bonus', trigger: 'purchase_completed', status: 'active', version: 3, modifiedAt: '2024-03-15' },
  { id: '2', name: 'Signup Welcome', trigger: 'user_signup', status: 'active', version: 1, modifiedAt: '2024-03-14' },
  { id: '3', name: 'Daily Login Streak', trigger: 'user_login', status: 'active', version: 5, modifiedAt: '2024-03-13' },
  { id: '4', name: 'Referral Reward', trigger: 'referral_completed', status: 'draft', version: 2, modifiedAt: '2024-03-12' },
  { id: '5', name: 'Premium Upgrade Bonus', trigger: 'subscription_upgraded', status: 'active', version: 1, modifiedAt: '2024-03-11' },
];

export const badges = [
  { id: '1', name: 'First Steps', description: 'Complete your profile', icon: '🚀', unlocks: 1250, color: '#8B5CF6' },
  { id: '2', name: 'Early Bird', description: 'Log in before 6 AM', icon: '🌅', unlocks: 342, color: '#F59E0B' },
  { id: '3', name: 'Shopaholic', description: 'Make 10 purchases', icon: '🛒', unlocks: 890, color: '#10B981' },
  { id: '4', name: 'Streak Master', description: '30 day login streak', icon: '🔥', unlocks: 156, color: '#EF4444' },
  { id: '5', name: 'Social Butterfly', description: 'Invite 5 friends', icon: '🦋', unlocks: 423, color: '#EC4899' },
  { id: '6', name: 'Power User', description: 'Use all features', icon: '⚡', unlocks: 89, color: '#6366F1' },
  { id: '7', name: 'Feedback Hero', description: 'Submit 3 reviews', icon: '💬', unlocks: 567, color: '#14B8A6' },
  { id: '8', name: 'Night Owl', description: 'Active after midnight', icon: '🦉', unlocks: 234, color: '#8B5CF6' },
  { id: '9', name: 'Milestone Master', description: 'Reach level 10', icon: '🏆', unlocks: 78, color: '#F59E0B' },
  { id: '10', name: 'Loyal Customer', description: '1 year member', icon: '💎', unlocks: 45, color: '#3B82F6' },
];

export const levels = [
  { id: '1', name: 'Bronze', tier: 'bronze', xpThreshold: 0, usersCount: 4520, color: '#CD7F32' },
  { id: '2', name: 'Silver', tier: 'silver', xpThreshold: 1000, usersCount: 2340, color: '#C0C0C0' },
  { id: '3', name: 'Gold', tier: 'gold', xpThreshold: 5000, usersCount: 890, color: '#FFD700' },
  { id: '4', name: 'Platinum', tier: 'platinum', xpThreshold: 15000, usersCount: 234, color: '#E5E4E2' },
  { id: '5', name: 'Diamond', tier: 'diamond', xpThreshold: 50000, usersCount: 45, color: '#B9F2FF' },
];

export const missions = [
  { id: '1', name: 'Welcome Journey', type: 'one-time', status: 'active', progress: 67, startDate: '2024-03-01', endDate: '2024-04-01' },
  { id: '2', name: 'Weekly Warrior', type: 'recurring', status: 'active', progress: 45, startDate: '2024-03-18', endDate: '2024-03-25' },
  { id: '3', name: 'Holiday Special', type: 'one-time', status: 'draft', progress: 0, startDate: '2024-04-15', endDate: '2024-04-30' },
];

export const leaderboardUsers = [
  { rank: 1, userId: 'usr_001', name: 'Alex Chen', score: 125400, avatar: 'AC' },
  { rank: 2, userId: 'usr_002', name: 'Sarah Miller', score: 118200, avatar: 'SM' },
  { rank: 3, userId: 'usr_003', name: 'James Wilson', score: 112800, avatar: 'JW' },
  { rank: 4, userId: 'usr_004', name: 'Emily Brown', score: 98500, avatar: 'EB' },
  { rank: 5, userId: 'usr_005', name: 'Michael Davis', score: 87300, avatar: 'MD' },
  { rank: 6, userId: 'usr_006', name: 'Lisa Anderson', score: 82100, avatar: 'LA' },
  { rank: 7, userId: 'usr_007', name: 'David Lee', score: 76400, avatar: 'DL' },
  { rank: 8, userId: 'usr_008', name: 'Jennifer Taylor', score: 71200, avatar: 'JT' },
  { rank: 9, userId: 'usr_009', name: 'Robert Martinez', score: 65800, avatar: 'RM' },
  { rank: 10, userId: 'usr_010', name: 'Amanda White', score: 61300, avatar: 'AW' },
];

export const recentActivity = [
  { id: '1', type: 'points', user: 'usr_001', action: 'earned 150 XP', timestamp: '2 min ago', icon: '⚡' },
  { id: '2', type: 'badge', user: 'usr_023', action: 'unlocked "First Steps" badge', timestamp: '5 min ago', icon: '🏅' },
  { id: '3', type: 'level', user: 'usr_089', action: 'reached Gold tier', timestamp: '12 min ago', icon: '🎖️' },
  { id: '4', type: 'mission', user: 'usr_045', action: 'completed "Welcome Journey"', timestamp: '18 min ago', icon: '🎯' },
  { id: '5', type: 'points', user: 'usr_112', action: 'earned 500 XP bonus', timestamp: '25 min ago', icon: '⚡' },
  { id: '6', type: 'streak', user: 'usr_067', action: 'achieved 7-day streak', timestamp: '32 min ago', icon: '🔥' },
  { id: '7', type: 'badge', user: 'usr_034', action: 'unlocked "Shopaholic" badge', timestamp: '41 min ago', icon: '🏅' },
  { id: '8', type: 'reward', user: 'usr_098', action: 'redeemed "Free Shipping"', timestamp: '55 min ago', icon: '🎁' },
];

export const eventsVolumeData = [
  { date: 'Mon', events: 12400 },
  { date: 'Tue', events: 14200 },
  { date: 'Wed', events: 13800 },
  { date: 'Thu', events: 15600 },
  { date: 'Fri', events: 18200 },
  { date: 'Sat', events: 16800 },
  { date: 'Sun', events: 14500 },
];

export const topRulesData = [
  { name: 'First Purchase', activations: 4520 },
  { name: 'Daily Login', activations: 3890 },
  { name: 'Signup Welcome', activations: 2340 },
  { name: 'Referral Reward', activations: 1890 },
  { name: 'Review Bonus', activations: 1230 },
];

export const eventTypesData = [
  { name: 'Purchase', value: 35, color: '#8B5CF6' },
  { name: 'Login', value: 28, color: '#6366F1' },
  { name: 'Signup', value: 15, color: '#10B981' },
  { name: 'Referral', value: 12, color: '#F59E0B' },
  { name: 'Other', value: 10, color: '#64748B' },
];

export const users = [
  { id: 'usr_001', email: 'alex.chen@email.com', level: 'Diamond', totalXp: 125400, lastActive: '2024-03-20' },
  { id: 'usr_002', email: 'sarah.miller@email.com', level: 'Platinum', totalXp: 48200, lastActive: '2024-03-20' },
  { id: 'usr_003', email: 'james.wilson@email.com', level: 'Gold', totalXp: 12800, lastActive: '2024-03-19' },
  { id: 'usr_004', email: 'emily.brown@email.com', level: 'Gold', totalXp: 9500, lastActive: '2024-03-18' },
  { id: 'usr_005', email: 'michael.davis@email.com', level: 'Silver', totalXp: 3300, lastActive: '2024-03-20' },
];

export const segments = [
  { id: '1', name: 'Power Users', criteria: 'XP > 10000 AND login_streak > 7', userCount: 1234 },
  { id: '2', name: 'New Users', criteria: 'created_at > 30 days ago', userCount: 4567 },
  { id: '3', name: 'At Risk', criteria: 'last_active > 14 days ago', userCount: 890 },
  { id: '4', name: 'Premium', criteria: 'subscription = premium', userCount: 2345 },
];

export const webhooks = [
  { id: '1', url: 'https://api.myapp.com/webhooks/gamification', events: ['points.awarded', 'badge.unlocked'], status: 'active', successRate: 99.2 },
  { id: '2', url: 'https://notifications.myapp.com/events', events: ['level.up', 'mission.completed'], status: 'active', successRate: 98.7 },
  { id: '3', url: 'https://analytics.myapp.com/track', events: ['*'], status: 'paused', successRate: 95.4 },
];

export const apiKeys = [
  { id: '1', name: 'Production API', environment: 'production', createdAt: '2024-01-15', lastUsed: '2024-03-20' },
  { id: '2', name: 'Sandbox Testing', environment: 'sandbox', createdAt: '2024-02-01', lastUsed: '2024-03-19' },
];

export const decisionLogs = [
  { id: 'dec_001', traceId: 'trace_abc123', userId: 'usr_001', timestamp: '2024-03-20 14:32:15', effects: ['awarded 150 XP', 'unlocked badge'] },
  { id: 'dec_002', traceId: 'trace_def456', userId: 'usr_023', timestamp: '2024-03-20 14:28:42', effects: ['awarded 50 XP'] },
  { id: 'dec_003', traceId: 'trace_ghi789', userId: 'usr_089', timestamp: '2024-03-20 14:25:18', effects: ['level up to Gold'] },
];

export const rewards = [
  { id: '1', name: 'Free Shipping', cost: 500, stock: 'unlimited', redemptions: 1234, image: '📦' },
  { id: '2', name: '10% Discount', cost: 1000, stock: 'unlimited', redemptions: 890, image: '🏷️' },
  { id: '3', name: 'Exclusive Merch', cost: 5000, stock: 50, redemptions: 23, image: '👕' },
  { id: '4', name: 'VIP Access', cost: 10000, stock: 10, redemptions: 5, image: '⭐' },
];
