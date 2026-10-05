# Auth URL Prefix Fix - API v1

## Problem

The frontend was calling auth endpoints without the `/api/v1` prefix, resulting in 404 errors.

**Incorrect URLs:**
- ❌ `POST /auth/login`
- ❌ `POST /auth/register`
- ❌ `GET /auth/me`
- ❌ `POST /auth/refresh`
- ❌ `POST /auth/logout`

**Correct URLs (Backend Routes):**
- ✅ `POST /api/v1/auth/login`
- ✅ `POST /api/v1/auth/register`
- ✅ `GET /api/v1/auth/me`
- ✅ `POST /api/v1/auth/refresh`
- ✅ `POST /api/v1/auth/logout`

## Solution

Updated all auth API calls to include the `/api/v1` prefix.

## Files Changed

### 1. `/src/services/api/auth.ts`
Updated all 5 auth endpoints:

```typescript
// Before
return api.post<AuthResponse>('/auth/login', data, { skipAuth: true });
return api.post<AuthResponse>('/auth/register', data, { skipAuth: true });
return api.get<AuthUser>('/auth/me');
return api.post<AuthResponse>('/auth/refresh');
return api.post<{ message: string }>('/auth/logout');

// After
return api.post<AuthResponse>('/api/v1/auth/login', data, { skipAuth: true });
return api.post<AuthResponse>('/api/v1/auth/register', data, { skipAuth: true });
return api.get<AuthUser>('/api/v1/auth/me');
return api.post<AuthResponse>('/api/v1/auth/refresh');
return api.post<{ message: string }>('/api/v1/auth/logout');
```

### 2. `/src/contexts/AuthContext.tsx`
Updated 4 auth endpoint calls:

**Line ~103:** Token verification
```typescript
// Before
const response = await api.get<AuthUser>('/auth/me', { showErrorToast: false });

// After
const response = await api.get<AuthUser>('/api/v1/auth/me', { showErrorToast: false });
```

**Line ~133:** Token refresh
```typescript
// Before
const response = await api.post<AuthResponse>('/auth/refresh', undefined, {
  showErrorToast: false
});

// After
const response = await api.post<AuthResponse>('/api/v1/auth/refresh', undefined, {
  showErrorToast: false
});
```

**Line ~152:** Login
```typescript
// Before
const response = await api.post<AuthResponse>('/auth/login', loginData, {
  skipAuth: true,
  showErrorToast: false
});

// After
const response = await api.post<AuthResponse>('/api/v1/auth/login', loginData, {
  skipAuth: true,
  showErrorToast: false
});
```

**Line ~174:** Register
```typescript
// Before
const response = await api.post<AuthResponse>('/auth/register', registerData, {
  skipAuth: true,
  showErrorToast: false
});

// After
const response = await api.post<AuthResponse>('/api/v1/auth/register', registerData, {
  skipAuth: true,
  showErrorToast: false
});
```

**Line ~190:** Logout
```typescript
// Before
await api.post<{ message: string }>('/auth/logout', undefined, {
  showErrorToast: false
});

// After
await api.post<{ message: string }>('/api/v1/auth/logout', undefined, {
  showErrorToast: false
});
```

## Backend Routes (Verified)

From `php artisan route:list | grep auth`:

```
POST  api/v1/auth/login       api.v1.auth.login     › Api\V1\AuthController@login
POST  api/v1/auth/logout      api.v1.auth.logout    › Api\V1\AuthController@logout
GET   api/v1/auth/me          api.v1.auth.me        › Api\V1\AuthController@me
POST  api/v1/auth/refresh     api.v1.auth.refresh   › Api\V1\AuthController@refresh
POST  api/v1/auth/register    api.v1.auth.register  › Api\V1\AuthController@register
```

## Why Use Custom Auth Endpoints?

### ✅ Use `/api/v1/auth/login` (Current Setup)

**Advantages:**
1. **No client secret in browser** - More secure for SPAs
2. **Custom token format** - Already returns `AuthTokenData`
3. **Flexible auth logic** - Tenant checks, roles, rate limiting
4. **Simpler flow** - Direct token issuance
5. **Better control** - Custom response format

### ❌ Don't Use `/oauth/token` for Browser SPAs

**Why not:**
- Requires `client_secret` in browser (security risk)
- Or requires backend proxy (unnecessary complexity)
- Less control over response format
- Passport grant complexity not needed for SPAs

**When to use `/oauth/token`:**
- Server-to-server authentication
- Trusted backend services
- Mobile apps with secure storage

## Authentication Flow

### 1. Login
```typescript
POST https://api.levelupos.ge/api/v1/auth/login
{
  "email": "user@example.com",
  "password": "password"
}

Response:
{
  "access_token": "eyJ0eXAiOiJKV1QiLCJhbGc...",
  "token_type": "Bearer",
  "expires_in": 31536000,
  "user": { /* user data */ }
}
```

### 2. Store Token
```typescript
localStorage.setItem('levelupos_token', access_token);
```

### 3. Use Token
```typescript
headers: {
  'Authorization': `Bearer ${token}`
}
```

### 4. Refresh Token
```typescript
POST https://api.levelupos.ge/api/v1/auth/refresh
Authorization: Bearer {old_token}

Response:
{
  "access_token": "new_token...",
  /* same format as login */
}
```

### 5. Logout
```typescript
POST https://api.levelupos.ge/api/v1/auth/logout
Authorization: Bearer {token}
```

## Backend Implementation

### LoginUserAction (Verified)
```php
// app/Domain/Auth/Actions/LoginUserAction.php
public function execute(LoginData $data): ?AuthTokenData
{
    $user = User::where('email', $data->email)->first();

    if (!$user || !Hash::check($data->password, $user->password)) {
        return null;
    }

    // Create Passport personal access token
    $tokenResult = $user->createToken('auth_token');
    $token = $tokenResult->accessToken;

    return AuthTokenData::from([
        'access_token' => $token,
        'token_type' => 'Bearer',
        'expires_in' => $tokenResult->token->expires_at
            ? $tokenResult->token->expires_at->diffInSeconds(now())
            : null,
        'user' => UserData::from($user),
    ]);
}
```

### AuthController (Verified)
```php
// app/Http/Controllers/Api/V1/AuthController.php
public function login(LoginRequest $request): JsonResponse
{
    $authToken = $this->loginUserAction->execute(
        LoginData::from($request->validated())
    );

    if (!$authToken) {
        return response()->json([
            'message' => 'Invalid credentials'
        ], 401);
    }

    return response()->json($authToken);
}
```

## Testing

### 1. Start Backend
```bash
cd /Users/saba/Projects/LevelUpOS
composer run dev
```

### 2. Start Frontend
```bash
cd /Users/saba/Projects/levelupos-portal
npm run dev
```

### 3. Test Login
1. Go to http://localhost:5173/login
2. Enter credentials
3. Check Network tab in DevTools:
   - Request URL: `https://api.levelupos.ge/api/v1/auth/login` ✅
   - Response should be 200 with token
   - Authorization header on subsequent requests

### 4. Verify All Auth Endpoints
```bash
# Check routes exist
php artisan route:list | grep "api/v1/auth"

# Should show:
# POST   api/v1/auth/login
# POST   api/v1/auth/logout
# GET    api/v1/auth/me
# POST   api/v1/auth/refresh
# POST   api/v1/auth/register
```

## CORS Configuration

Ensure CORS is configured for auth endpoints:

```php
// config/cors.php
'paths' => [
    'api/*',           // Covers /api/v1/auth/*
    'login',
    'logout',
    'oauth/*',
],
```

## Build Status

✅ Frontend build successful
✅ No linting errors
✅ All auth endpoints updated
✅ Ready for deployment

## Summary

| Endpoint | Old URL | New URL | Status |
|----------|---------|---------|--------|
| Login | `/auth/login` | `/api/v1/auth/login` | ✅ Fixed |
| Register | `/auth/register` | `/api/v1/auth/register` | ✅ Fixed |
| Get User | `/auth/me` | `/api/v1/auth/me` | ✅ Fixed |
| Refresh | `/auth/refresh` | `/api/v1/auth/refresh` | ✅ Fixed |
| Logout | `/auth/logout` | `/api/v1/auth/logout` | ✅ Fixed |

All auth endpoints now correctly use the `/api/v1` prefix matching the backend routes.
