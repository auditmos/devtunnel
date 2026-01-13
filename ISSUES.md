# 1
curl http://localhost:8080/health
should return JSON
ok, time, version

# 2 rown in devtunnel

When you run ./devtunnel start 3000:
requests table: YES — rows are inserted here. Each HTTP request that goes through the tunnel is logged with a tunnel_id reference.
tunnels table: NO — rows are not inserted here. The table exists in the schema, but the code doesn't populate it.
Looking at the code in cmd/devtunnel/main.go (line 244), a tunnelID is generated:
tunnelID := ulid.Make().String()
This tunnelID is used in the requests table, but there's no corresponding code that creates a record in the tunnels table with the tunnel's metadata (subdomain, server_url, started_at, etc.).
So currently:
The tunnels table exists but remains empty
The requests table is populated with request logs that reference a tunnel_id
This looks like an incomplete feature — the schema supports tunnel tracking, but the implementation doesn't create tunnel records.

# 3 rows in server

When devtunnel server runs, it creates server.db at ~/.devtunnel/server.db with three tables:
tunnels table (from base schema)
Columns: id, subdomain, server_url, started_at, ended_at, status
requests table (from base schema)
Columns: id, tunnel_id, timestamp, method, url, request_headers, request_body, status_code, response_headers, response_body, duration_ms, created_at
shared_blobs table (from blob schema)
Columns: id, ciphertext, created_at, expires_at
What Actually Gets Inserted
When you run ./devtunnel server:
tunnels table: NO — rows are not inserted. The table exists but isn't populated.
requests table: NO — rows are not inserted. The server doesn't log requests; only clients log to their own devtunnel.db.
shared_blobs table: YES — rows are inserted when someone uses the share feature. This happens when:
A client dashboard calls /api/share/{request_id}
The dashboard sends encrypted request data to the server's /api/share endpoint
The server saves it to shared_blobs with a 24-hour expiration
Summary
tunnels: Empty (not used)
requests: Empty (not used on server)
shared_blobs: Populated only when the share feature is used
The tunnels and requests tables exist in the schema but aren't used by the server. Only shared_blobs is actively used.

# 4 malformed

curl -H "Host: http://1b4cdd38.localhost" http://localhost:8080
400 Bad Request: malformed Host header%

should work the same way as:

curl http://localhost:8080/proxy/1b4cdd38/

AND 

curl -H "Host: 1b4cdd38.localhost" http://localhost:8080

# 5 Share Request Securely

when visit http://localhost:4040/
select request
click on it share securly
copied address does not have port number if run localhost:
http://localhost/shared/01KEVJWTWYABBC29XQXXD8Q4MD#LHtvvwdh1Co6snmdtwt56kJPITpHKRWNV_FeLEo8RT4=
probably should be
http://localhost:8080/shared/01KEVJWTWYABBC29XQXXD8Q4MD#LHtvvwdh1Co6snmdtwt56kJPITpHKRWNV_FeLEo8RT4=

# 6 shared view

when visit http://localhost:8080/shared/01KEVJWTWYABBC29XQXXD8Q4MD#LHtvvwdh1Co6snmdtwt56kJPITpHKRWNV_FeLEo8RT4=
there should be small info added
```
<div class="info">
    To replay this request: <code>devtunnel replay {{current_url}} --port 3000</code>
</div>
```

# 7 secrets not scrubbed

curl -X POST http://localhost:8080/1b4cdd38 \
  -H "Authorization: Bearer secret-token-abc123" \
  -H "X-API-Key: my-secret-api-key-xyz789" \
  -H "Cookie: session=abc123def456; auth=secret" \
  -H "X-CSRF-Token: csrf-token-12345" \
  -H "X-Auth-Token: auth-token-secret" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -H "User-Agent: curl/7.68.0" \
  -d '{"test": "data", "password": "should-not-be-scrubbed-in-body"}'

  curl -X GET "http://localhost:8080/proxy/fca36bee?param1=value1&param2=value2" \
  -H "Authorization: Bearer secret-token-abc123" \
  -H "X-API-Key: my-secret-api-key-xyz789" \
  -H "Cookie: session=abc123def456; auth=secret" \
  -H "X-CSRF-Token: csrf-token-12345" \
  -H "X-Auth-Token: auth-token-secret" \
  -H "Accept: application/json" \
  -H "User-Agent: curl/7.68.0"