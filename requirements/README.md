# Requirements

One markdown file per requirement / initiative. The filename encodes the
authoring date and the current status so the directory is scannable at a
glance without opening each file.

## Filename convention

```text
<YYYY-MM-DD>_<slug>_<STATUS>.md
```

- `YYYY-MM-DD` — the date the requirement was authored. Stable; it does
  not change as work progresses.
- `slug` — short kebab-case topic, e.g. `agentsview-platform-upgrade`.
- `STATUS` — current lifecycle state (uppercase), one of:

  | Status    | Meaning                                                     |
  | --------- | ----------------------------------------------------------- |
  | `PLANNED` | Written and agreed, not started.                            |
  | `WIP`     | Implementation in progress.                                 |
  | `DONE`    | Delivered and verified.                                     |
  | `BLOCKED` | Started but stalled on an external dependency or decision.  |
  | `DROPPED` | Decided not to do; kept for the record.                     |

## Updating status

Status lives in the filename, so changing it is a rename that keeps the
date and slug stable:

```bash
git mv requirements/2026-06-12_agentsview-platform-upgrade_PLANNED.md \
       requirements/2026-06-12_agentsview-platform-upgrade_WIP.md
```

Keeping the date and slug fixed means a requirement's history stays easy
to follow across `git log --follow`.
