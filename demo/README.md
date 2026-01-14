```
pnpm install
pnpm run dev
```

```
open http://localhost:3000
```

```
# Create user
curl -X POST http://localhost:3000/admin/users \
  -H "Authorization: Bearer super-secret-token-here" \
  -H "Content-Type: application/json" \
  -d '{"name": "John Doe", "email": "john@example.com"}'

# Delete user
curl -X DELETE http://localhost:3000/admin/users/1 \
  -H "Authorization: Bearer super-secret-token-here"
```