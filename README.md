## env
DB_URL
GRPC_ADDR
REDIS_ADDR
SECRET_KEY


## schema 
User login (initial issuance)
Client -> Gateway
Client posts credentials to Gateway (e.g., POST /login).
Gateway -> Auth
Gateway forwards credentials to Auth (or calls Auth gRPC).
Auth
Auth validates credentials.
Auth generates:
access_token (JWT, short TTL e.g. 15m)
refresh_token (raw random secret, long TTL e.g. 30d)
Auth returns both tokens (raw refresh_token and access_token) to Gateway.
Gateway
Gateway computes refresh_hash = SHA256(refresh_token) (hex or base64).
Gateway writes Redis key for the refresh token hash (example pattern below).
Gateway sets cookie (or response) to client:
Set refresh_token into a Secure, HttpOnly, SameSite cookie (long TTL).
Send access_token to client in header or ephemeral cookie as you prefer.
Redis (written by Gateway)
Key: refresh:th:<refresh_hash>
Value: HASH fields: user_id, device_id (optional), issued_at, meta (optional)
TTL: EXPIRE set to refreshTTL (e.g., 30243600 seconds)
Example Redis commands (Gateway):
HSET refresh:th:<hash> user_id <uid> device_id <did> issued_at <ts> meta '{}'
EXPIRE refresh:th:<hash> <seconds>