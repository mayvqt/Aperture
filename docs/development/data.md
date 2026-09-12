# Data and schema

`internal/db/schema.go` is the schema and revision source of truth.
`PRAGMA user_version` records the revision. The database contains settings,
templates, invites, registrations, audit events, sessions, webhooks, and managed
users. Secret settings, retained invite tokens, access tokens, and webhook URLs
are encrypted with the installation encryption key; hashed identifiers are used
where raw tokens are unnecessary.

`internal/db/store.go` owns connection policy: one connection, WAL mode, foreign
keys, a five-second busy timeout, private directories, and mode `0600` database
files. Keep SQLite operations in `internal/db`; handlers must not become a second
schema or query owner.

## Schema changes

Migrations are forward-only and transactional. Add a new revision instead of
rewriting the meaning of an applied revision. Prefer expand/contract evolution:
add compatible columns or tables, deploy readers/writers that tolerate the
transition, and remove obsolete data only in a later explicitly supported step.
Startup applies every outstanding supported revision in order and rejects
databases newer than the binary.

Every schema change requires:

- a fresh-database test;
- an upgrade test starting at every affected supported revision;
- preservation tests for existing rows and encrypted values;
- failure/rollback-boundary coverage; and
- corresponding backup and release guidance when operator action is required.

Never repair an upgrade by deleting the database. Back up and restore the whole
state set as described in [Operations](operations.md).

Revision 5 adds durable account cleanup state. Expiry is captured in the invite
reservation transaction and retained through failures and recovery. Migration
preserves existing deadlines and reconstructs missing finite deadlines from the
original registration time and retained invite duration. Incomplete accounts
with known upstream IDs enter cleanup independently of access retry attempts.

Revision 6 adds server ownership using provider, normalized URL and the authenticated
server ID. Existing invite, registration and tracked-user rows retain their data
with unknown ownership until an administrator reviews each record. Connection
publication atomically saves settings and revokes sessions on an origin change.
Reassigning an account preserves its deadline and resets the old server's disable
acknowledgement so expired access is checked again.
