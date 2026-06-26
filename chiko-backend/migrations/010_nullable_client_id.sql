-- Migration 010: make chats.client_id nullable
--
-- Why: CreateChat (Step 1.5) supports "pending" flow where a chat is created
-- with client_phone_pending when the client hasn't registered yet.
-- The original NOT NULL constraint blocked this INSERT at runtime.
-- LinkPendingChats (called during Bootstrap) fills client_id when client signs in.
--
-- Side effect: UNIQUE(producer_id, client_id) now allows NULL client_id.
-- In PostgreSQL, NULLs are NOT equal in unique constraints, so a producer can
-- have multiple pending chats (different phone numbers) without violating uniqueness.
-- This is correct: each pending chat has a distinct client_phone_pending.

ALTER TABLE chats
    ALTER COLUMN client_id DROP NOT NULL;
