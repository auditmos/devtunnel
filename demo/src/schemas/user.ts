import { z } from 'zod';

/**
 * Schema for a single user
 */
export const UserSchema = z.object({
  id: z.number(),
  name: z.string(),
  email: z.email(),
  createdAt: z.string(),
});

export type User = z.infer<typeof UserSchema>;

/**
 * Schema for users list response (GET /users)
 */
export const UsersResponseSchema = z.array(UserSchema);

export type UsersResponse = z.infer<typeof UsersResponseSchema>;

/**
 * Schema for creating a new user (POST /admin/users)
 */
export const CreateUserRequestSchema = z.object({
  name: z.string().min(1),
  email: z.email(),
});

export type CreateUserRequest = z.infer<typeof CreateUserRequestSchema>;
