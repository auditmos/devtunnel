import { z } from 'zod';
import { ValidationError } from './error-handling.js';

type ValidationContext = 'request' | 'response';

/**
 * Validates data against a Zod schema.
 * Throws ValidationError with details if validation fails.
 */
function validate<T extends z.ZodType>(
    schema: T,
    data: unknown,
    context: ValidationContext
): z.infer<T> {
    const result = schema.safeParse(data);
    if (!result.success) {
        const errors = result.error.issues
            .map((issue) => `${issue.path.join('.')}: ${issue.message}`)
            .join(', ');
        throw new ValidationError(`Invalid ${context}: ${errors}`, 'VALIDATION_ERROR');
    }
    return result.data;
}

/**
 * Validates request data against a Zod schema.
 */
export function validateRequest<T extends z.ZodType>(schema: T, data: unknown): z.infer<T> {
    return validate(schema, data, 'request');
}

/**
 * Validates response data against a Zod schema.
 */
export function validateResponse<T extends z.ZodType>(schema: T, data: unknown): z.infer<T> {
    return validate(schema, data, 'response');
}
