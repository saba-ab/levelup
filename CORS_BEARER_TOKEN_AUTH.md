# CORS Configuration - Bearer Token Authentication

## Important: Authentication Method Matters!

This application uses **Bearer Token authentication** (Laravel Passport), NOT cookie-based authentication.

## ⚠️ Common Mistake: Using `credentials: 'include'`

**DO NOT** use `credentials: 'include'` with Bearer token authentication!

### Why Not?

- `credentials: 'include'` is ONLY for **cookie-based authentication** (sessions, CSRF tokens)
- Bearer tokens are sent via the `Authorization` header, NOT cookies
- Using `credentials: 'include'` with Bearer tokens can cause CORS issues

### The Correct Approach

#### Frontend (levelupos-portal)

**✅ Correct - Bearer Token (Current Implementation):**

```typescript
// useApi.ts
const token = localStorage.getItem('levelupos_token');

const headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'Authorization': `Bearer ${token}`, // Token in header
};

fetch(url, {
  method: 'POST',
  headers,
  body: JSON.stringify(data),
  // NO credentials: 'include' - we're using tokens, not cookies!
});
```

**❌ Wrong - Don't mix Bearer tokens with credentials:**

```typescript
// DON'T DO THIS!
fetch(url, {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`, // Token auth
  },
  credentials: 'include', // ❌ Cookie auth - CONFLICT!
});
```

#### Backend (LevelUpOS)

**✅ Correct - Bearer Token CORS Config:**

```php
// config/cors.php
return [
    'paths' => ['api/*', 'login', 'logout', 'oauth/*'],
    'allowed_methods' => ['*'],
    'allowed_origins' => [
        env('FRONTEND_URL'),
        env('TEMP_FRONTEND_URL'),
    ],
    'allowed_headers' => ['*'],
    'exposed_headers' => [],
    'max_age' => 3600,
    'supports_credentials' => false, // ✅ FALSE for Bearer tokens
];
```

**❌ Wrong - Don't enable credentials for Bearer tokens:**

```php
// DON'T DO THIS with Bearer tokens!
'supports_credentials' => true, // ❌ Only for cookies
```

## When to Use Each Method

### Bearer Token Authentication (Current Setup)
**Use when:**
- Building a stateless API
- Mobile apps
- SPA (Single Page Applications)
- Multi-domain setups

**Configuration:**
```typescript
// Frontend
headers: { 'Authorization': `Bearer ${token}` }
// NO credentials: 'include'
```
```php
// Backend
'supports_credentials' => false
```

### Cookie-Based Authentication (Not Used Here)
**Use when:**
- Traditional server-rendered apps
- Need CSRF protection
- Session-based auth

**Configuration:**
```typescript
// Frontend
credentials: 'include'
// NO Authorization header
```
```php
// Backend
'supports_credentials' => true
```

## Current Implementation (Verified)

### ✅ Frontend Configuration

**File: `src/hooks/useApi.ts`**
```typescript
// Get auth token from localStorage
const token = !skipAuth ? localStorage.getItem('levelupos_token') : null;

// Build headers
const headers: Record<string, string> = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-Requested-With': 'XMLHttpRequest',
};

if (token) {
  headers['Authorization'] = `Bearer ${token}`; // ✅ Bearer token
}

// Fetch request
const response = await fetch(config.url, {
  ...config,
  signal: abortControllerRef.current.signal,
  // ✅ NO credentials: 'include'
});
```

### ✅ Backend Configuration

**File: `config/cors.php`**
```php
return [
    'paths' => ['api/*', 'login', 'logout', 'oauth/*'],
    'allowed_methods' => ['*'],
    'allowed_origins' => [
        env('FRONTEND_URL'),
        env('TEMP_FRONTEND_URL'),
    ],
    'allowed_headers' => ['*'],
    'exposed_headers' => [],
    'max_age' => 3600,
    'supports_credentials' => false, // ✅ Correct for Bearer tokens
];
```

## Authentication Flow

### Login Process
1. User submits credentials to `/api/v1/auth/login`
2. Backend returns access token in JSON response
3. Frontend stores token in `localStorage`
4. Frontend sends token in `Authorization` header for subsequent requests

```typescript
// Login
const response = await api.post('/api/v1/auth/login', {
  email: 'user@example.com',
  password: 'password',
});

// Store token
localStorage.setItem('levelupos_token', response.data.access_token);

// Use token in requests
headers['Authorization'] = `Bearer ${token}`;
```

### Logout Process
1. Frontend calls `/api/v1/auth/logout` with token
2. Backend revokes the token
3. Frontend removes token from `localStorage`

```typescript
// Logout
await api.post('/api/v1/auth/logout');
localStorage.removeItem('levelupos_token');
```

## Troubleshooting

### CORS Error: "credentials mode is 'include'"
**Problem:** You set `credentials: 'include'` but backend has `supports_credentials => false`

**Solution:** Remove `credentials: 'include'` from frontend

### Token Not Being Sent
**Problem:** Authorization header missing

**Solution:**
1. Check token exists: `localStorage.getItem('levelupos_token')`
2. Verify header is set: Check Network tab → Request Headers
3. Ensure `skipAuth: false` in API options

### 401 Unauthorized
**Problem:** Token invalid or expired

**Solution:**
1. Check token format: Should be `Bearer {token}`
2. Verify token hasn't expired
3. Re-authenticate if needed

## Testing

### Check Current Setup
```bash
# 1. Start backend
cd /Users/saba/Projects/LevelUpOS
composer run dev

# 2. Start frontend
cd /Users/saba/Projects/levelupos-portal
npm run dev

# 3. Open browser DevTools → Network tab
# 4. Login and check requests:
#    - Authorization header should be present: "Bearer xxx"
#    - NO Cookie header
#    - CORS should work without errors
```

### Verify in Browser DevTools

**Request Headers (should include):**
```
Authorization: Bearer eyJ0eXAiOiJKV1QiLCJhbGc...
Accept: application/json
Content-Type: application/json
```

**Request Headers (should NOT include):**
```
Cookie: ... ❌ Not needed for Bearer tokens
```

## Migration Guide (If You Need to Switch)

### From Cookie Auth to Bearer Token (Current)
```diff
- credentials: 'include'
+ // Remove credentials
+ headers: { 'Authorization': `Bearer ${token}` }
```

```diff
- 'supports_credentials' => true
+ 'supports_credentials' => false
```

### From Bearer Token to Cookie Auth (Not Recommended)
```diff
- headers: { 'Authorization': `Bearer ${token}` }
+ credentials: 'include'
```

```diff
- 'supports_credentials' => false
+ 'supports_credentials' => true
```

## Resources

- [Laravel Passport Documentation](https://laravel.com/docs/12.x/passport)
- [Laravel CORS Documentation](https://laravel.com/docs/12.x/cors)
- [MDN: Authorization Header](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization)
- [MDN: Fetch credentials](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch#sending_a_request_with_credentials_included)

## Summary

✅ **Current Setup (Correct):**
- Frontend: Bearer token in `Authorization` header
- Backend: `supports_credentials => false`
- No `credentials: 'include'`

This is the correct configuration for stateless API authentication with Laravel Passport.
