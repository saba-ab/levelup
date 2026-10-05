# API Routes Constants - Centralized Endpoint Management

## Overview

Refactored all hardcoded API endpoint URLs into a centralized constants file for better maintainability and consistency.

## The Problem

Before this refactor, API endpoints were hardcoded throughout the codebase:

```typescript
// ❌ Hardcoded URLs scattered everywhere
api.post('/api/v1/auth/login', data);
api.get('/api/v1/auth/me');
api.post('/api/v1/players', data);
```

**Issues:**
- Hard to change API version (would need to update every file)
- Prone to typos and inconsistencies
- Difficult to see all available endpoints at a glance
- No autocomplete/IntelliSense support

## The Solution

Created `/src/lib/api-routes.ts` with all API endpoint constants:

```typescript
// ✅ Centralized constants
export const API_VERSION = '/api/v1';

export const AUTH_ENDPOINTS = {
  LOGIN: `${API_VERSION}/auth/login`,
  REGISTER: `${API_VERSION}/auth/register`,
  LOGOUT: `${API_VERSION}/auth/logout`,
  ME: `${API_VERSION}/auth/me`,
  REFRESH: `${API_VERSION}/auth/refresh`,
} as const;

// Usage
import { AUTH_ENDPOINTS } from '@/lib/api-routes';
api.post(AUTH_ENDPOINTS.LOGIN, data);
```

## Benefits

### 1. **Single Source of Truth**
Change the API version in one place:
```typescript
// Update once, applies everywhere
export const API_VERSION = '/api/v2';
```

### 2. **Type Safety**
TypeScript autocomplete for all endpoints:
```typescript
AUTH_ENDPOINTS.  // IntelliSense shows: LOGIN, REGISTER, LOGOUT, ME, REFRESH
```

### 3. **No Typos**
Can't misspell endpoints:
```typescript
// ❌ Would fail at compile time
api.post(AUTH_ENDPOINTS.LOGN, data);  // Error: Property 'LOGN' does not exist
```

### 4. **Dynamic Routes**
Functions for parameterized endpoints:
```typescript
export const PLAYER_ENDPOINTS = {
  SHOW: (id: number | string) => `${API_VERSION}/players/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/players/${id}`,
} as const;

// Usage
api.get(PLAYER_ENDPOINTS.SHOW(123));  // '/api/v1/players/123'
```

### 5. **Easy to Audit**
All API endpoints in one file - easy to see what's available

## Available Endpoint Groups

### Authentication
```typescript
AUTH_ENDPOINTS.LOGIN
AUTH_ENDPOINTS.REGISTER
AUTH_ENDPOINTS.LOGOUT
AUTH_ENDPOINTS.ME
AUTH_ENDPOINTS.REFRESH
```

### Players
```typescript
PLAYER_ENDPOINTS.LIST
PLAYER_ENDPOINTS.CREATE
PLAYER_ENDPOINTS.SHOW(id)
PLAYER_ENDPOINTS.UPDATE(id)
PLAYER_ENDPOINTS.DELETE(id)
PLAYER_ENDPOINTS.WALLET(id)
PLAYER_ENDPOINTS.WALLET_TRANSACTIONS(id)
```

### Badges
```typescript
BADGE_ENDPOINTS.LIST
BADGE_ENDPOINTS.CREATE
BADGE_ENDPOINTS.SHOW(id)
BADGE_ENDPOINTS.UPDATE(id)
BADGE_ENDPOINTS.DELETE(id)
BADGE_ENDPOINTS.AWARD
BADGE_ENDPOINTS.REVOKE
```

### Missions
```typescript
MISSION_ENDPOINTS.LIST
MISSION_ENDPOINTS.CREATE
MISSION_ENDPOINTS.SHOW(id)
MISSION_ENDPOINTS.UPDATE(id)
MISSION_ENDPOINTS.DELETE(id)
MISSION_ENDPOINTS.START
MISSION_ENDPOINTS.UPDATE_PROGRESS
MISSION_ENDPOINTS.COMPLETE
```

### Levels
```typescript
LEVEL_ENDPOINTS.LIST
LEVEL_ENDPOINTS.CREATE
LEVEL_ENDPOINTS.SHOW(id)
LEVEL_ENDPOINTS.UPDATE(id)
LEVEL_ENDPOINTS.DELETE(id)
LEVEL_ENDPOINTS.GRANT_XP
```

### Rewards
```typescript
REWARD_ENDPOINTS.LIST
REWARD_ENDPOINTS.CREATE
REWARD_ENDPOINTS.SHOW(id)
REWARD_ENDPOINTS.UPDATE(id)
REWARD_ENDPOINTS.DELETE(id)
REWARD_ENDPOINTS.CLAIM
REWARD_ENDPOINTS.REDEEM
```

### Streaks
```typescript
STREAK_ENDPOINTS.LIST
STREAK_ENDPOINTS.CREATE
STREAK_ENDPOINTS.SHOW(id)
STREAK_ENDPOINTS.UPDATE(id)
STREAK_ENDPOINTS.DELETE(id)
STREAK_ENDPOINTS.RECORD_ACTIVITY
```

### Leaderboards
```typescript
LEADERBOARD_ENDPOINTS.LIST
LEADERBOARD_ENDPOINTS.CREATE
LEADERBOARD_ENDPOINTS.SHOW(id)
LEADERBOARD_ENDPOINTS.UPDATE(id)
LEADERBOARD_ENDPOINTS.DELETE(id)
LEADERBOARD_ENDPOINTS.ENTRIES(id)
LEADERBOARD_ENDPOINTS.PLAYER_RANK(leaderboardId, playerId)
```

### Wallets
```typescript
WALLET_ENDPOINTS.CREDIT
WALLET_ENDPOINTS.DEBIT
WALLET_ENDPOINTS.TRANSFER
```

### Events, Rules, Programs, Users
```typescript
EVENT_ENDPOINTS.*
RULE_ENDPOINTS.*
PROGRAM_ENDPOINTS.*
USER_ENDPOINTS.*
```

## Usage Examples

### Basic Usage
```typescript
import { AUTH_ENDPOINTS } from '@/lib/api-routes';

// Login
const response = await api.post(AUTH_ENDPOINTS.LOGIN, { email, password });

// Get current user
const user = await api.get(AUTH_ENDPOINTS.ME);
```

### With Parameters
```typescript
import { PLAYER_ENDPOINTS } from '@/lib/api-routes';

// Get specific player
const player = await api.get(PLAYER_ENDPOINTS.SHOW(123));

// Update player
await api.put(PLAYER_ENDPOINTS.UPDATE(123), updatedData);

// Get player's wallet
const wallet = await api.get(PLAYER_ENDPOINTS.WALLET(123));
```

### Multiple Endpoints
```typescript
import {
  AUTH_ENDPOINTS,
  PLAYER_ENDPOINTS,
  BADGE_ENDPOINTS
} from '@/lib/api-routes';

// Login
await api.post(AUTH_ENDPOINTS.LOGIN, credentials);

// Fetch data
const players = await api.get(PLAYER_ENDPOINTS.LIST);
const badges = await api.get(BADGE_ENDPOINTS.LIST);
```

## Files Refactored

### 1. `/src/services/api/auth.ts`
**Before:**
```typescript
api.post('/api/v1/auth/login', data);
api.post('/api/v1/auth/register', data);
api.get('/api/v1/auth/me');
api.post('/api/v1/auth/refresh');
api.post('/api/v1/auth/logout');
```

**After:**
```typescript
import { AUTH_ENDPOINTS } from '@/lib/api-routes';

api.post(AUTH_ENDPOINTS.LOGIN, data);
api.post(AUTH_ENDPOINTS.REGISTER, data);
api.get(AUTH_ENDPOINTS.ME);
api.post(AUTH_ENDPOINTS.REFRESH);
api.post(AUTH_ENDPOINTS.LOGOUT);
```

### 2. `/src/contexts/AuthContext.tsx`
Updated all 5 auth endpoint calls to use `AUTH_ENDPOINTS` constants.

## Adding New Endpoints

When adding new API endpoints:

### 1. Add to Constants
```typescript
// src/lib/api-routes.ts
export const NEW_FEATURE_ENDPOINTS = {
  LIST: `${API_VERSION}/new-feature`,
  CREATE: `${API_VERSION}/new-feature`,
  SHOW: (id: number | string) => `${API_VERSION}/new-feature/${id}`,
} as const;
```

### 2. Use in Code
```typescript
import { NEW_FEATURE_ENDPOINTS } from '@/lib/api-routes';

const data = await api.get(NEW_FEATURE_ENDPOINTS.LIST);
```

## Changing API Version

If you need to change the API version:

### Option 1: Global Change
```typescript
// src/lib/api-routes.ts
export const API_VERSION = '/api/v2';  // Changed from /api/v1
```

All endpoints automatically update!

### Option 2: Gradual Migration
```typescript
export const API_V1 = '/api/v1';
export const API_V2 = '/api/v2';

export const AUTH_ENDPOINTS = {
  LOGIN: `${API_V2}/auth/login`,  // Migrated to v2
  REGISTER: `${API_V1}/auth/register`,  // Still on v1
} as const;
```

## Testing

### Check All Endpoints
```typescript
import { getAllEndpoints } from '@/lib/api-routes';

// Get all endpoint strings
const endpoints = getAllEndpoints();
console.log(endpoints);
// ['/api/v1/auth/login', '/api/v1/auth/register', ...]
```

### Verify in Browser
1. Start dev server: `npm run dev`
2. Open DevTools → Network tab
3. Make API calls
4. Verify URLs match constants:
   - Should see: `https://api.levelupos.ge/api/v1/auth/login` ✅

## Build Status

✅ No linting errors
✅ Build successful
✅ Type-safe constants
✅ All auth endpoints refactored
✅ Ready for further refactoring

## Next Steps

### Recommended: Refactor Other Services

Apply the same pattern to other API service files:

1. `src/services/api/players.ts` → Use `PLAYER_ENDPOINTS`
2. `src/services/api/badges.ts` → Use `BADGE_ENDPOINTS`
3. `src/services/api/missions.ts` → Use `MISSION_ENDPOINTS`
4. `src/services/api/mechanics.ts` → Use appropriate endpoint constants

### Example Pattern
```typescript
// src/services/api/players.ts
import { PLAYER_ENDPOINTS } from '@/lib/api-routes';

export function usePlayerService() {
  const api = useApi();

  const getPlayers = useCallback(async (filters?: PlayerFilters) => {
    return api.get(PLAYER_ENDPOINTS.LIST, {
      params: filters
    });
  }, [api]);

  const getPlayer = useCallback(async (id: number) => {
    return api.get(PLAYER_ENDPOINTS.SHOW(id));
  }, [api]);

  // ... more methods
}
```

## Benefits Summary

| Benefit | Before | After |
|---------|--------|-------|
| **Maintainability** | Change 20+ files | Change 1 file |
| **Type Safety** | String typos possible | Compile-time checks |
| **Discoverability** | Grep through code | One file to reference |
| **Consistency** | Manual consistency | Automatic consistency |
| **Refactoring** | Error-prone | Safe with IDE support |

## Conclusion

This refactor significantly improves code quality and maintainability by:
- ✅ Centralizing all API endpoints
- ✅ Adding type safety
- ✅ Making version changes trivial
- ✅ Improving developer experience
- ✅ Reducing errors and inconsistencies

All changes are backward compatible and the frontend builds successfully!
