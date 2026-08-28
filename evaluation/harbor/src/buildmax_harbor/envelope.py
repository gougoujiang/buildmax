"""Reading BuildMax's print-mode result envelope.

Separate from agent.py, and importing nothing from Harbor, so the mapping from
a BuildMax run to a Harbor trial can be tested without a benchmark harness.
The envelope's shape is documented in docs/reference/cli.md.
"""

import hashlib
from pathlib import Path
from typing import Any

# From internal/interface/cli/exit_code.go, where the codes are a documented
# contract. Only the cap is named: it is the one code this adapter treats
# differently, because it is the agent's own answer rather than a fault.
EXIT_ITERATION_CAP = 7

_NANO_UNITS_PER_UNIT = 1_000_000_000


def digest_file(path: Path) -> str:
    """Return the artifact identity BuildMax records for a binary.

    The prefixed form matches what evaluation/adapter writes for a local trial,
    so a Harbor subject and a CLI subject name the same file the same way.
    """
    h = hashlib.sha256()
    with path.open("rb") as f:
        # Chunked: the CLI is tens of megabytes, and a trial should not hold a
        # second copy of it in memory to name it.
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return "sha256:" + h.hexdigest()


def cost_usd(usage: dict[str, Any]) -> float | None:
    """Convert BuildMax's integer cost, when it is priced and in dollars.

    A model with no pricing configured reports no cost, and reporting zero for
    it would read as free rather than as unpriced. A cost in another currency is
    dropped for the same reason: Harbor's field is dollars, and putting a
    different currency's number in it would be a wrong figure rather than a
    missing one.
    """
    cost = usage.get("cost")
    if not isinstance(cost, dict):
        return None
    total = cost.get("total")
    if isinstance(total, bool) or not isinstance(total, int):
        return None
    if cost.get("currency") != "USD":
        return None
    return total / _NANO_UNITS_PER_UNIT


def trial_metadata(envelope: dict[str, Any]) -> dict[str, Any]:
    """The facts about a run that Harbor's own result model has no field for."""
    error = envelope.get("error") or {}
    return {
        "exit_code": envelope.get("exit_code"),
        "error_kind": error.get("kind"),
        "error_message": error.get("message"),
        "session_id": envelope.get("session_id"),
        "trace_id": envelope.get("trace_id"),
        "model": envelope.get("model"),
        "workspace": envelope.get("workspace"),
        "tool_calls": envelope.get("tool_calls"),
        "duration_ms": envelope.get("duration_ms"),
        # Stated rather than left for a later reader to infer from exit_code: an
        # attempt that stopped on its budget is not a capability result, and
        # docs/design/evaluation-system.md section 7.4 keeps those apart.
        "stopped_at_iteration_cap": envelope.get("exit_code") == EXIT_ITERATION_CAP,
    }
