"""Tests for reading BuildMax's print-mode result envelope."""

from buildmax_harbor.envelope import EXIT_ITERATION_CAP, cost_usd, trial_metadata


def test_cost_converts_nano_units_to_dollars():
    assert cost_usd({"cost": {"currency": "USD", "total": 1_234_567_890}}) == 1.23456789


def test_an_unpriced_model_reports_no_cost():
    # Not zero: zero reads as free, and a model with no pricing configured is
    # unpriced rather than free.
    assert cost_usd({}) is None
    assert cost_usd({"cost": None}) is None


def test_another_currency_is_dropped_rather_than_relabelled():
    assert cost_usd({"cost": {"currency": "CNY", "total": 1_000_000_000}}) is None


def test_the_iteration_cap_is_stated_not_inferred():
    meta = trial_metadata(
        {
            "exit_code": EXIT_ITERATION_CAP,
            "error": {"kind": "iteration_cap", "message": "max iterations exceeded"},
            "trace_id": "abc",
        }
    )
    assert meta["stopped_at_iteration_cap"] is True
    assert meta["error_kind"] == "iteration_cap"
    assert meta["trace_id"] == "abc"


def test_a_completed_run_is_not_marked_capped():
    meta = trial_metadata({"exit_code": 0, "tool_calls": 12})
    assert meta["stopped_at_iteration_cap"] is False
    assert meta["error_kind"] is None
    assert meta["tool_calls"] == 12
