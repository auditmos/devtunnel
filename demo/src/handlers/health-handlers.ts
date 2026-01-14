import type { Context } from 'hono';
import { HealthCheckResponseSchema } from '../schemas/health.js';
import { validateResponse } from '../utils/validation.js';

/**
 * Handler for GET /
 */
export function healthCheckHandler(c: Context) {
  const response = {
    status: 'ok',
    service: 'devtunnel-demo',
    time: new Date().toISOString(),
    message: 'Service healthy',
    version: '0.0.1',
  };
  const validatedResponse = validateResponse(HealthCheckResponseSchema, response);
  return c.json(validatedResponse);
}

