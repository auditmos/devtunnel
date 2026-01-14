import type { User, CreateUserRequest } from '../schemas/user.js';
import { ApiError } from '../utils/error-handling.js';

/**
 * Mock user data - in a real app, this would come from a database
 */
const mockUsers: User[] = [
  {
    id: 1,
    name: 'Alice Johnson',
    email: 'alice@example.com',
    createdAt: '2024-01-15T10:30:00Z',
  },
  {
    id: 2,
    name: 'Bob Smith',
    email: 'bob@example.com',
    createdAt: '2024-02-20T14:45:00Z',
  },
  {
    id: 3,
    name: 'Charlie Brown',
    email: 'charlie@example.com',
    createdAt: '2024-03-10T09:15:00Z',
  },
];

let nextId = 4;

/**
 * Fetches all users
 */
export function getAllUsers(): User[] {
  return mockUsers;
}

/**
 * Creates a new user
 */
export function createUser(data: CreateUserRequest): User {
  const newUser: User = {
    id: nextId++,
    name: data.name,
    email: data.email,
    createdAt: new Date().toISOString(),
  };
  mockUsers.push(newUser);
  return newUser;
}

/**
 * Deletes a user by ID
 */
export function deleteUser(id: number): void {
  const index = mockUsers.findIndex((user) => user.id === id);
  if (index === -1) {
    throw new ApiError(`User with id ${id} not found`, 404, 'NOT_FOUND');
  }
  mockUsers.splice(index, 1);
}
