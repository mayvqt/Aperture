# Documentation conventions

Write for one audience and one task per page. Keep commands executable, link to
the source of truth instead of copying it, and describe only shipped behavior.
Unfinished work belongs in a dedicated roadmap only when it has an accepted
scope; confirmed defects belong in a known-issues page; release notes describe
one shipped audience-facing revision. Move facts between those pages rather than
duplicating them.

For large, high-risk, or external-integration work, keep the accepted plan
immutable and record execution/evidence separately. Inventory each operation,
verify upstream contracts against canonical sources, use deterministic fakes,
and make real-service smoke tests explicit and opt-in. Record only reproduced
defects.

## Documentation impact map

| Change | Update in the same batch |
| --- | --- |
| User-visible behavior | Nearest user page, [Capabilities](../capabilities.md), and [README](../../README.md) if navigation changes |
| Configuration or setup | [Configuration](../configuration.md), [Setup](../setup.md), relevant deployment page, and their nearest index |
| Package ownership or architecture | [Codebase map](codebase-map.md), [Architecture](architecture.md), and this index |
| Schema, retention, encryption, or stored data | [Data](data.md), [Operations](operations.md) when operator impact changes, and nearest index |
| Tests, tools, UI states, or CI | [Validation](validation.md) and nearest index |
| Security boundary or disclosure process | [Security](../SECURITY.md), relevant architecture/data/operations page, and nearest index |
| Deployment, backup, health, rollback, or release | [Operations](operations.md), relevant public setup/configuration page, and [README](../../README.md) |

Update this map when a new canonical documentation domain is introduced.
