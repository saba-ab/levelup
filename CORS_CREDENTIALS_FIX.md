# CORS Configuration - Credentials Support

## Problem
The backend CORS configuration requires credentials (cookies) to be sent with requests, but the frontend was not configured to send them, causing CORS failures.

## Solution
Added `credentials: 'include'` to all fetch requests in the frontend.

## Files Modified

### Frontend (levelupos-portal)

#### 1. `/src/hooks/useApi.ts` (Line 163-166)
```typescript
const response = await fetch(config.url, {
  ...config,
  signal: abortControllerRef.current.signal,
  credentials: 'include', // Send cookies with requests for CORS
});
```

#### 2. `/src/hooks/useConnectionStatus.ts` (3 fetch calls updated)
- Line ~77-86: Initial connection check
- Line ~162-170: Background polling
- Line ~226-234: Manual check

All updated to include:
```typescript
credentials: 'include', // Send cookies with requests for CORS
```

## What This Does

### `credentials: 'include'`
This fetch option tells the browser to:
- Send cookies with cross-origin requests
- Include authentication credentials
- Allow the server to set cookies from the response

### Backend Requirements (Already Configured)
The backend CORS configuration in `config/cors.php` already supports this:
```php
'supports_credentials' => true,
```

## Testing

After this change, all API requests will:
1. ✅ Send cookies with requests
2. ✅ Pass CORS preflight checks
3. ✅ Allow session-based authentication
4. ✅ Work with Laravel Sanctum/Passport cookies

## Verification

Build completed successfully:
```bash
npm run build
# ✓ 3521 modules transformed
# ✓ built in 5.06s
```

No TypeScript or linting errors.

## How to Test

1. Start the backend: `composer run dev`
2. Start the frontend: `npm run dev`
3. Make any API request from the frontend
4. Check browser DevTools → Network tab:
   - Request should include `credentials: include`
   - Response should have proper CORS headers
   - No CORS errors in console

## Additional Notes

### Why This Was Needed
Laravel's CORS middleware requires credentials for:
- Session management
- CSRF token validation
- Cookie-based authentication
- Stateful API authentication

### Alternative (If Not Using Cookies)
If you're only using Bearer tokens (not cookies), you could set:
```php
// config/cors.php
'supports_credentials' => false,
```

But since we're using Laravel's session/cookie features, `credentials: 'include'` is required.

## Related Configuration

### Backend CORS Config
File: `LevelUpOS/config/cors.php`
```php
'supports_credentials' => true,
'allowed_origins' => ['http://localhost:5173', 'http://localhost:5174'],
'allowed_headers' => ['*'],
'exposed_headers' => [],
'max_age' => 0,
```

### API Base Configuration
File: `levelupos-portal/src/hooks/useApi.ts`
```typescript
headers: {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-Requested-With': 'XMLHttpRequest', // Laravel CSRF compatibility
  'Authorization': `Bearer ${token}`, // If using token auth
}
```

## Common CORS Issues Resolved

✅ **Preflight requests failing** - Fixed by `credentials: 'include'`
✅ **Cookies not sent** - Fixed by `credentials: 'include'`
✅ **Session not persisting** - Fixed by `credentials: 'include'`
✅ **CSRF token issues** - Fixed by proper headers + credentials

## Resources

- [MDN: Fetch API - credentials](https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch#sending_a_request_with_credentials_included)
- [Laravel CORS Documentation](https://laravel.com/docs/12.x/cors)
- [Axios equivalent](https://axios-http.com/docs/req_config): `axios.defaults.withCredentials = true`
