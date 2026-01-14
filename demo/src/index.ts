import { serve } from '@hono/node-server'
import { Hono } from 'hono'
import { errorHandler, onErrorHandler } from './middleware/error-handler.js'
import { bearerAuth } from './middleware/auth.js'
import { healthCheckHandler } from './handlers/health-handlers.js'
import { getUsersHandler } from './handlers/user-handlers.js'
import { createUserHandler, deleteUserHandler } from './handlers/admin-handlers.js'

const App = new Hono()

App.onError(onErrorHandler);

App.use('*', errorHandler());

// Public routes
App.get('/', healthCheckHandler);
App.get('/users', getUsersHandler);

// Admin routes (protected)
App.use('/admin/*', bearerAuth());
App.post('/admin/users', createUserHandler);
App.delete('/admin/users/:id', deleteUserHandler);

const PORT = process.env.PORT ? Number(process.env.PORT) : 3000;

serve(
  { fetch: App.fetch, port: PORT },
  (info) => {
    console.info(`🚀 Server started and listening on http://localhost:${info.port} (PID: ${process.pid})`);
  }
);