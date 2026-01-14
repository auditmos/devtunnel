import type { Context, Next } from 'hono';
import { ApiError, AuthenticationError } from '../utils/error-handling.js';

/**
 * Bearer token authentication middleware
 * Validates the Authorization header against ADMIN_TOKEN from environment
 */
export function bearerAuth() {
    return async (c: Context, next: Next) => {
        const authHeader = c.req.header('Authorization');

        if (!authHeader) {
            throw new AuthenticationError('Authorization header required', 401, 'UNAUTHORIZED');
        }

        if (!authHeader.startsWith('Bearer ')) {
            throw new AuthenticationError('Invalid authorization format. Use: Bearer <token>', 401, 'UNAUTHORIZED');
        }

        const token = authHeader.slice(7);
        const adminToken = process.env.ADMIN_TOKEN;

        if (!adminToken) {
            throw new ApiError('Server misconfigured: ADMIN_TOKEN not set', 500, 'CONFIG_ERROR');
        }

        if (token !== adminToken) {
            throw new AuthenticationError('Invalid token', 403, 'FORBIDDEN');
        }

        await next();
    };
}
