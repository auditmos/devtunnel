import type { Context } from 'hono';
import { CreateUserRequestSchema, UserSchema } from '../schemas/user.js';
import { createUser, deleteUser } from '../services/user-service.js';
import { validateRequest, validateResponse } from '../utils/validation.js';

/**
 * Handler for POST /admin/users
 * Creates a new user
 */
export async function createUserHandler(c: Context) {
    const body = await c.req.json();
    const data = validateRequest(CreateUserRequestSchema, body);
    const user = createUser(data);
    const validatedResponse = validateResponse(UserSchema, user);
    return c.json(validatedResponse, 201);
}

/**
 * Handler for DELETE /admin/users/:id
 * Deletes a user by ID
 */
export function deleteUserHandler(c: Context) {
    const id = Number(c.req.param('id'));
    if (Number.isNaN(id)) {
        return c.json({ error: 'Invalid user ID' }, 400);
    }
    deleteUser(id);
    return c.json({ message: `User ${id} deleted` });
}
