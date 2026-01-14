import type { Context } from 'hono';
import { UsersResponseSchema } from '../schemas/user.js';
import { getAllUsers } from '../services/user-service.js';
import { validateResponse } from '../utils/validation.js';

/**
 * Handler for GET /users
 * Returns a JSON array of all users
 */
export function getUsersHandler(c: Context) {
  const users = getAllUsers();
  const validatedResponse = validateResponse(UsersResponseSchema, users);
  return c.json(validatedResponse);
}
