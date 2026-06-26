-- Migration 011: remove overly permissive guest_sessions RLS policy.
--
-- guest_sessions_anon_select USING (true) allowed ANY anonymous user to read
-- ALL guest carts (cart_jsonb with product lists, producer_id). Data leak.
--
-- The Go backend accesses guest_sessions via service_role (bypasses RLS),
-- so this policy was never needed for backend reads.
-- Guest-facing endpoints (GET /api/guest/cart/:id) are protected at the
-- application layer — callers must supply the session UUID.

DROP POLICY IF EXISTS guest_sessions_anon_select ON guest_sessions;

-- Keep anon INSERT so guests can create sessions via the Go backend proxy.
-- All access to guest_sessions is via service_role in the Go layer.
