"""Tests for the trial home's settings file.

They import no Harbor code on purpose: the credential rendering has to be
checkable without a benchmark harness installed.
"""

import pytest

from buildmax_harbor.settings import (
    DEFAULT_PROVIDER,
    PROTOCOLS,
    map_provider,
    model_id,
    render_settings,
    validate_pricing,
    validate_protocol,
    validate_reasoning,
)


def test_native_providers_keep_their_protocol():
    assert map_provider("anthropic") == "anthropic"
    assert map_provider("openai") == "openai"
    assert map_provider("ollama") == "ollama"


def test_an_unknown_provider_is_openai_compatible():
    # A gateway BuildMax has never heard of still speaks Chat Completions, and
    # calling that openai_compatible is the accurate protocol name rather than a
    # guess about the vendor.
    assert map_provider("openrouter") == DEFAULT_PROVIDER
    assert map_provider(None) == DEFAULT_PROVIDER


# Harbor is given `<provider>/<model>` and splits it on the first slash. A
# gateway's model id has slashes of its own, and taking the last segment would
# send a name the endpoint has never heard of — a failure that reads as a bad
# model rather than as an adapter that mangled it.
def test_only_the_provider_prefix_is_stripped():
    assert model_id("anthropic/claude-opus-4-7") == "claude-opus-4-7"
    assert model_id("openrouter/openai/gpt-5.6-luna") == "openai/gpt-5.6-luna"
    assert model_id("openrouter/z-ai/glm-5.3") == "z-ai/glm-5.3"


def test_a_model_named_without_a_provider_is_left_alone():
    assert model_id("gpt-5.6-luna") == "gpt-5.6-luna"


def test_a_named_protocol_overrides_the_slug():
    # A gateway serves more than one protocol, so which one a trial speaks is a
    # choice: OpenRouter answers Chat Completions, and it also fronts the
    # vendors' own shapes. Inference picks one; naming one settles it.
    assert PROTOCOLS == {"anthropic", "openai", "ollama", DEFAULT_PROVIDER}
    assert validate_protocol("openai") == "openai"
    assert validate_protocol(None) is None
    # A protocol BuildMax does not implement would be refused by the CLI inside
    # the container, one task at a time.
    with pytest.raises(ValueError, match="Invalid provider"):
        validate_protocol("openrouter")


def test_reasoning_outside_buildmax_vocabulary_is_refused():
    assert validate_reasoning("high") == "high"
    assert validate_reasoning(None) is None
    # Harbor's leaderboard uses these; silently running them as "high" would
    # record a subject that cannot be reproduced from its own manifest.
    for level in ("xhigh", "max"):
        with pytest.raises(ValueError, match="reasoning_effort"):
            validate_reasoning(level)


def test_settings_carry_only_the_model_entry():
    rendered = render_settings(
        model="claude-opus-4-7",
        provider="anthropic",
        base_url="https://api.anthropic.com/v1",
        api_key="secret",
        reasoning="high",
    )
    assert rendered.splitlines() == [
        "log_level: error",
        "models:",
        '  - model: "claude-opus-4-7"',
        '    name: "claude-opus-4-7"',
        '    provider: "anthropic"',
        '    api_url: "https://api.anthropic.com/v1"',
        '    api_key: "secret"',
        '    reasoning: "high"',
    ]
    # No hooks, plugins, permissions, or sandbox block. Anything here that the
    # subject did not ask for is something a result cannot be attributed to.
    for absent in ("hooks", "tools", "plugins", "sandbox"):
        assert absent not in rendered


def test_pricing_is_rendered_as_quoted_decimals():
    rendered = render_settings(
        model="m",
        provider="anthropic",
        api_key="secret",
        pricing={
            "currency": "USD",
            "input_per_mtok": "0.2",
            "cache_read_per_mtok": "0.02",
            "output_per_mtok": "1.2",
        },
    )
    # The rates keep the order BuildMax declares them in, not the caller's.
    assert rendered.splitlines()[-5:] == [
        "    pricing:",
        '      currency: "USD"',
        '      input_per_mtok: "0.2"',
        '      cache_read_per_mtok: "0.02"',
        '      output_per_mtok: "1.2"',
    ]
    # A rate the caller did not give is absent, not zero: BuildMax reads an
    # empty rate as free, which is a real price on some providers and a wrong
    # one here.
    assert "cache_write_per_mtok" not in rendered


def test_a_rate_given_as_a_number_is_still_quoted():
    # Harbor's --ak parses JSON, so a caller writing 0.2 hands over a float.
    # Written bare it would come back through YAML as a float too, and a rate
    # in millionths loses its last digits that way.
    rendered = render_settings(
        model="m",
        provider="anthropic",
        api_key="secret",
        pricing={"currency": "USD", "input_per_mtok": 0.2},
    )
    assert '      input_per_mtok: "0.2"' in rendered


def test_an_unpriced_run_renders_no_pricing_block():
    assert "pricing" not in render_settings(
        model="m", provider="anthropic", api_key="secret"
    )


def test_pricing_that_would_report_a_wrong_number_is_refused():
    assert validate_pricing(None) is None
    # No currency: rates that cannot be added to anything.
    with pytest.raises(ValueError, match="currency"):
        validate_pricing({"input_per_mtok": "0.2"})
    # A rate that is not a number would price the run as something.
    with pytest.raises(ValueError, match="not a number"):
        validate_pricing({"currency": "USD", "input_per_mtok": "cheap"})
    # A misspelled rate is silently dropped by a permissive reader, and the
    # total is then quietly wrong by whatever that rate was worth.
    with pytest.raises(ValueError, match="unknown pricing field"):
        validate_pricing({"currency": "USD", "input_per_million": "0.2"})


def test_an_openai_compatible_provider_needs_an_endpoint():
    # Without a base URL the run would go to whatever the binary defaults to,
    # which is a different subject than the one the manifest would claim. The
    # key is supplied because a home missing both is reported as missing the
    # key: that is the one the reader has to fix first.
    with pytest.raises(ValueError, match="needs an endpoint"):
        render_settings(model="some-model", provider=DEFAULT_PROVIDER, api_key="secret")


def test_a_home_without_a_key_is_refused():
    # Harbor hands back a provider's default endpoint only once it has resolved
    # that provider's key, so an unresolved key reaches here as an unset base
    # URL. Reported as a missing endpoint it sends the reader to set the wrong
    # variable — and only after every trial container is already built, since
    # the CLI is what refuses a keyless home.
    with pytest.raises(ValueError, match="needs an api_key"):
        render_settings(model="m", provider="anthropic")
    with pytest.raises(ValueError, match="needs an api_key"):
        render_settings(
            model="m", provider=DEFAULT_PROVIDER, base_url="https://gateway.example/v1"
        )


def test_a_local_provider_needs_no_key():
    # A local daemon authenticates nothing: demanding a placeholder would refuse
    # a setup that works.
    rendered = render_settings(model="gemma4:e2b", provider="ollama")
    assert "api_key" not in rendered


def test_a_key_with_yaml_syntax_in_it_stays_one_scalar():
    rendered = render_settings(
        model="m",
        provider="anthropic",
        api_key='sk-a"b\nc: d',
    )
    # The escaped form is one line, so a key holding a quote and a newline
    # cannot close its scalar and inject settings of its own.
    key_lines = [line for line in rendered.splitlines() if "api_key" in line]
    assert key_lines == ['    api_key: "sk-a\\"b\\nc: d"']


def test_optional_numbers_are_omitted_rather_than_zeroed():
    rendered = render_settings(model="m", provider="anthropic", api_key="secret")
    assert "context_window" not in rendered
    assert "max_tokens" not in rendered
    assert "reasoning" not in rendered
