"""BuildMax's custom-Agent adapter for the Harbor benchmark harness.

Deliberately empty of re-exports. Harbor is told the full path
``buildmax_harbor.agent:Buildmax``, and importing the agent here would drag the
harness into every import of the two harness-free modules beside it — which are
harness-free so they can be tested without one.
"""

# How this adapter invokes the subject. It changes when the invocation changes,
# because that moves results without the product moving, and a comparison
# spanning one is not paired.
#
# Stated here rather than read from ../pins.json: an installed wheel has no
# repository beside it. pins.json stays the authority and a Go test holds this
# constant to it.
ADAPTER_VERSION = 1
