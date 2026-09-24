# Design

Add `POST /transactions/revoke-allocation`, distinct from the whole-source revoke. Accept allocation ID, optional expected transaction ID, idempotency key, and safe return target; derive owner, obligation, tenant, and amount from stored rows. In one DB transaction lock the source, validate and conditionally void the one confirmed rent allocation, lock its obligation/room, reproject paid/status from current allocations and cash receipts, write an action linked by operation ID, then reproject the source from all remaining effective allocations. Check affected-row count. No schema migration is expected.

The review evidence model carries allocation ID alongside source ID. The UI owns dialog presentation; the server owns mutation validation. Existing source-wide revoke remains available from its separate preview flow.
