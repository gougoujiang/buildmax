"""The settings.yaml a trial home carries.

Separate from agent.py, and importing nothing from Harbor, because this is the
file the provider credential is written into: it has to be testable without a
benchmark harness installed.
"""

import json
from typing import Any

# BuildMax reasoning levels, from internal/config/config.go. Harbor's own
# leaderboard vocabulary is wider — it carries xhigh and max — and an unknown
# level is refused rather than mapped down: a subject recorded as running at
# "max" that actually ran at "high" cannot be re-run against its own result.
REASONING_LEVELS = frozenset({"off", "low", "medium", "high"})

# Harbor provider slug -> BuildMax wire protocol, from
# internal/core/llm/provider.go. A BuildMax provider names a protocol, not a
# vendor, so anything reached over OpenAI Chat Completions is openai_compatible
# whoever serves it.
PROVIDERS = {
    "anthropic": "anthropic",
    "openai": "openai",
    "ollama": "ollama",
}
DEFAULT_PROVIDER = "openai_compatible"


def map_provider(slug: str | None) -> str:
    """Map a Harbor provider slug onto the protocol BuildMax speaks to it."""
    return PROVIDERS.get(slug or "", DEFAULT_PROVIDER)


def model_id(model_name: str) -> str:
    """Strip the provider prefix Harbor added, and only that one.

    Harbor is given ``<provider>/<model>`` and splits it on the first slash.
    The model half can hold slashes of its own — a gateway names models
    ``openai/gpt-5.6-luna``, and that whole string is the id the endpoint
    expects. Taking the last segment instead of the first split would send
    ``gpt-5.6-luna`` to a gateway that has never heard of it, so the trial would
    fail at the provider with an error about the model rather than about the
    adapter that mangled it.
    """
    return model_name.split("/", 1)[-1]


def validate_reasoning(level: str | None) -> str | None:
    if level is None or level in REASONING_LEVELS:
        return level
    raise ValueError(
        f"Invalid reasoning_effort {level!r} for buildmax. "
        f"Valid values: {', '.join(sorted(REASONING_LEVELS))}"
    )


PRICING_RATES = (
    "input_per_mtok",
    "cache_read_per_mtok",
    "cache_write_per_mtok",
    "output_per_mtok",
)


def validate_pricing(pricing: dict[str, Any] | None) -> dict[str, Any] | None:
    """Check a price list before a trial runs on it.

    An unpriced run reports its cost as unavailable, which is honest. A run
    priced with a currency nobody named, or with a rate that is not a number,
    reports a figure that looks exact and is not — and it would do so after the
    money was already spent. So the shape is checked up front.
    """
    if pricing is None:
        return None
    if not isinstance(pricing, dict):
        raise ValueError(f"pricing must be a JSON object, got {type(pricing).__name__}")
    if not pricing.get("currency"):
        raise ValueError("pricing needs a currency; rates with no currency price nothing")
    unknown = set(pricing) - {"currency", *PRICING_RATES}
    if unknown:
        raise ValueError(
            f"unknown pricing field(s) {sorted(unknown)}; "
            f"valid rates are {', '.join(PRICING_RATES)}"
        )
    for rate in PRICING_RATES:
        value = pricing.get(rate)
        if value is None:
            continue
        try:
            float(value)
        except (TypeError, ValueError):
            raise ValueError(f"pricing.{rate} is not a number: {value!r}") from None
    return pricing


def render_settings(
    *,
    model: str,
    provider: str,
    base_url: str | None = None,
    api_key: str | None = None,
    context_window: int | None = None,
    max_tokens: int | None = None,
    reasoning: str | None = None,
    pricing: dict[str, Any] | None = None,
) -> str:
    """Render the one model entry a trial home holds.

    Nothing else goes in it. docs/design/evaluation-system.md section 2.1 lists
    local settings, hooks, plugins, and permissions among the things that
    silently change what a benchmark measures, and a home holding only this has
    none of them to inherit. The key set mirrors the settingsModel struct in
    evaluation/adapter/home.go, and a test there holds this file to it.
    """
    if not model:
        raise ValueError("a trial home needs a model")
    if provider == DEFAULT_PROVIDER and not base_url:
        raise ValueError(
            f"provider {provider!r} needs an endpoint. Set the provider's base-url "
            "environment variable, or use a provider BuildMax speaks natively: "
            f"{', '.join(sorted(PROVIDERS))}"
        )

    lines = [
        # Trial stderr is diagnostic output Harbor keeps; info-level logs would
        # bury a real failure in ordinary progress.
        "log_level: error",
        "models:",
        f"  - model: {_yaml_str(model)}",
        f"    name: {_yaml_str(model)}",
        f"    provider: {_yaml_str(provider)}",
    ]
    if base_url:
        lines.append(f"    api_url: {_yaml_str(base_url)}")
    if api_key:
        lines.append(f"    api_key: {_yaml_str(api_key)}")
    if context_window:
        lines.append(f"    context_window: {int(context_window)}")
    if max_tokens:
        lines.append(f"    max_tokens: {int(max_tokens)}")
    if reasoning:
        lines.append(f"    reasoning: {_yaml_str(reasoning)}")
    if pricing:
        # Rates are quoted, because BuildMax parses them as decimals: a price
        # written as a bare YAML number becomes a float on the way in, and a
        # rate in millionths loses its last digits to that.
        lines.append("    pricing:")
        lines.append(f"      currency: {_yaml_str(pricing['currency'])}")
        for rate in PRICING_RATES:
            if pricing.get(rate) is not None:
                lines.append(f"      {rate}: {_yaml_str(str(pricing[rate]))}")
    return "\n".join(lines) + "\n"


def _yaml_str(value: Any) -> str:
    """Quote a scalar as YAML.

    A JSON string is a valid YAML double-quoted scalar, so this is exact rather
    than a hand-rolled escape — and that matters: an API key is one of the
    values going through it.
    """
    return json.dumps(value)
