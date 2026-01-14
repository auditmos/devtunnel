import { z } from 'zod';

/**
 * Schema for health check response (GET /)
 */
export const HealthCheckResponseSchema = z.object({
    status: z.string(),
    service: z.string(),
    time: z.string(),
    message: z.string(),
    version: z.string(),
  });
  
  export type HealthCheckResponse = z.infer<typeof HealthCheckResponseSchema>;
  