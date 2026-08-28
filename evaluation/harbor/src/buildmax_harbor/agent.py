"""The BuildMax agent Harbor loads to run a task.

Harbor installs this into the task container and calls it in headless mode. It
runs the shipped CLI binary and nothing else: no library call, no second copy of
the agent loop, and no re-grading of what the task's own verifier decides. See
docs/design/evaluation-system.md sections 7.3 and 14.2.

Written against the Harbor release named in ../pins.json. It reaches one
underscore-prefixed helper on the base class, ``_upload_config_text``, which is
only safe because that version is pinned exactly; re-read the base class before
moving the pin.
"""

import json
import shlex
from pathlib import Path
from typing import Any, override

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.agents.model_connection import ModelConnectionSpec
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

from buildmax_harbor import ADAPTER_VERSION
from buildmax_harbor.envelope import (
    EXIT_ITERATION_CAP,
    cost_usd,
    digest_file,
    trial_metadata,
)
from buildmax_harbor.settings import (
    map_provider,
    model_id,
    render_settings,
    validate_pricing,
    validate_reasoning,
)


class Buildmax(BaseInstalledAgent):
    """Runs the built BuildMax CLI against a Harbor task.

    The binary is uploaded from the host rather than downloaded in the
    container. That is what makes the result a black-box measurement of a named
    artifact: the digest of the file that ran is known before the trial starts,
    and a task whose container has no network still measures the same subject.
    Build one with ``./make build cli linux/amd64``.
    """

    # No default provider. Harbor infers one from the ``provider/model`` name it
    # was given, and BuildMax has no house model to fall back to.
    MODEL_CONNECTION = ModelConnectionSpec()

    _REMOTE_HOME = "/tmp/buildmax-home"
    _REMOTE_UPLOAD = "/tmp/buildmax.upload"
    _REMOTE_BIN = "/usr/local/bin/buildmax"
    _RESULT_FILENAME = "buildmax-result.json"
    _SESSIONS_DIRNAME = "sessions"

    def __init__(
        self,
        *args: Any,
        binary: str | Path | None = None,
        reasoning_effort: str | None = None,
        max_iterations: int | None = None,
        context_window: int | None = None,
        max_tokens: int | None = None,
        pricing: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> None:
        super().__init__(*args, **kwargs)

        if binary is None:
            raise ValueError(
                "The buildmax kwarg 'binary' is required: it is the built CLI this "
                "trial measures. Produce one with `./make build cli linux/amd64` and "
                "pass --ak binary=bin/buildmax-linux-amd64"
            )
        self._binary = Path(binary)
        if not self._binary.is_file():
            raise FileNotFoundError(f"buildmax binary not found: {self._binary}")
        # Digested here, by the only code that has the file. A result that named
        # a source revision rather than the artifact that ran would not be a
        # black-box measurement, and a caller asserting the digest afterwards
        # can assert the wrong one: nothing downstream can check it.
        self._artifact_digest = digest_file(self._binary)

        self._reasoning_effort = validate_reasoning(reasoning_effort)
        # Validated here rather than at first use: a malformed price list should
        # stop the job before it starts, not after 89 tasks have been paid for.
        self._pricing = validate_pricing(pricing)
        self._max_iterations = max_iterations
        self._context_window = context_window
        self._max_tokens = max_tokens

    @staticmethod
    @override
    def name() -> str:
        return "buildmax"

    @override
    def get_version_command(self) -> str | None:
        return f"{self._REMOTE_BIN} --version"

    @override
    def parse_version(self, stdout: str) -> str:
        # `buildmax version 1.2.3 (abc1234)`. The commit is kept: an untagged
        # build reports "dev", and two dev builds are not the same subject.
        return stdout.strip().removeprefix("buildmax version").strip()

    @override
    async def install(self, environment: BaseEnvironment) -> None:
        # curl and gnupg are deliberately absent: the binary is uploaded, not
        # fetched. git and ripgrep are what the agent's own tools reach for, and
        # ca-certificates is what lets it reach the model at all.
        # procps is not for the agent's own use: it is what lets the cleanup
        # below reap a run Harbor cancelled. See _collect_evidence.
        await self.ensure_system_dependencies(
            environment, ("bash", "git", "ripgrep", "ca_certificates", "procps")
        )
        await environment.upload_file(self._binary, self._REMOTE_UPLOAD)
        # install(1) rather than mv: it sets the mode in the same step, so there
        # is no window where the binary exists and is not executable.
        await self.exec_as_root(
            environment,
            command=(
                f"install -m 0755 {shlex.quote(self._REMOTE_UPLOAD)} "
                f"{shlex.quote(self._REMOTE_BIN)} && "
                f"rm -f {shlex.quote(self._REMOTE_UPLOAD)}"
            ),
        )

    @override
    @with_prompt_template
    async def run(
        self, instruction: str, environment: BaseEnvironment, context: AgentContext
    ) -> None:
        agent_dir = self.environment_logs_dir.as_posix()
        result_path = (self.environment_logs_dir / self._RESULT_FILENAME).as_posix()

        # HOME is set alongside BUILDMAX_HOME because a container's default user
        # may have none, and a run whose HOME is unset writes its stray state to
        # whatever the shell decided instead.
        env = {"BUILDMAX_HOME": self._REMOTE_HOME, "HOME": self._REMOTE_HOME}

        await self.exec_as_agent(
            environment,
            command=(
                f"mkdir -p {shlex.quote(self._REMOTE_HOME)} {shlex.quote(agent_dir)}"
            ),
            env=env,
        )
        await self._upload_config_text(
            environment,
            content=self._settings_yaml(),
            remote_path=f"{self._REMOTE_HOME}/settings.yaml",
            filename="settings.yaml",
        )

        try:
            await self.exec_as_agent(
                environment, command=self._run_command(instruction, result_path), env=env
            )
        finally:
            await self._collect_evidence(environment, env)

    def _run_command(self, instruction: str, result_path: str) -> str:
        """The one shell command that is the trial.

        No ``--workspace``: the CLI defaults to the working directory, which is
        where Harbor put the task. Passing a path of our own would move the
        agent out of the environment the task built for it.
        """
        return (
            f"{shlex.quote(self._REMOTE_BIN)} "
            f"-p {shlex.quote(instruction)} "
            "--output json --no-stream "
            f"{self._run_flags()}"
            # No stdin: the run is unattended, and a binary that blocked on a
            # closed terminal would hang the trial to its timeout instead of
            # failing. stdout is the result envelope and goes to a file; stderr
            # stays on the exec so Harbor's own error patterns can classify a
            # provider failure into a retryable one.
            f"</dev/null >{shlex.quote(result_path)}\n"
            "code=$?\n"
            # One code is swallowed. Reaching the iteration cap is the agent
            # deciding to stop, not the harness breaking: letting it raise would
            # lose the verifier's verdict on work that really happened, and make
            # Harbor retry a budget that runs out at the same place.
            f'if [ "$code" -eq {EXIT_ITERATION_CAP} ]; then\n'
            '  echo "buildmax: stopped at its iteration cap" >&2\n'
            "  exit 0\n"
            "fi\n"
            'exit "$code"'
        )

    async def _collect_evidence(
        self, environment: BaseEnvironment, env: dict[str, str]
    ) -> None:
        """Reap the run, copy its traces out, then remove the home with the key.

        All three are best effort. The trial's outcome is what the verifier
        makes of the workspace, and losing the diagnostics must not turn a
        graded run into a harness failure.

        The reaping is what makes the task's own time budget mean anything.
        Harbor bounds the agent phase with `asyncio.wait_for`, which cancels
        this coroutine — but cancelling a coroutine that is awaiting a
        `docker compose exec` ends the wait, not the process: the environment
        terminates the client only on its own per-exec timeout, which nothing
        here sets. So without this the container keeps running the CLI after
        Harbor has given up on it, and a task with a 30-minute budget was
        observed still working, and still spending, at 98 minutes. A run that
        outlives its budget is not comparable with agents that were held to it.
        """
        # First, so the process stops writing before the traces are copied.
        try:
            await self.exec_as_agent(
                environment,
                # -x on the process name, not -f on the command line: the shell
                # running this very command has the binary's path in its own
                # command line, so a -f pattern would match and kill the
                # cleanup. `|| true` because on the ordinary path the run has
                # already exited and pkill finding nothing is not a failure.
                command="pkill -x buildmax || true",
                env=env,
            )
        except Exception:
            self.logger.warning("could not reap the buildmax run", exc_info=True)

        sessions_src = f"{self._REMOTE_HOME}/{self._SESSIONS_DIRNAME}"
        sessions_dst = (self.environment_logs_dir / self._SESSIONS_DIRNAME).as_posix()
        try:
            await self.exec_as_agent(
                environment,
                command=(
                    f"if [ -d {shlex.quote(sessions_src)} ]; then\n"
                    f"  rm -rf {shlex.quote(sessions_dst)}\n"
                    f"  cp -R {shlex.quote(sessions_src)} {shlex.quote(sessions_dst)}\n"
                    "fi"
                ),
                env=env,
            )
        except Exception:
            self.logger.warning("could not collect buildmax traces", exc_info=True)

        # The home holds the provider credential. Removing it bounds how long a
        # key the task's own commands could read stays on the filesystem.
        try:
            await self.exec_as_agent(
                environment,
                command=f"rm -rf {shlex.quote(self._REMOTE_HOME)}",
                env=env,
            )
        except Exception:
            self.logger.warning("could not remove the buildmax home", exc_info=True)

    @override
    def populate_context_post_run(self, context: AgentContext) -> None:
        envelope = self._read_envelope()
        if envelope is None:
            return

        usage = envelope.get("usage") or {}
        context.n_input_tokens = usage.get("total_prompt")
        # BuildMax breaks cache out of the prompt total rather than adding to
        # it, which is what Harbor's "input including cache" already means, so
        # the two line up without arithmetic.
        context.n_cache_tokens = usage.get("total_cache_read")
        context.n_output_tokens = usage.get("total_completion")
        context.cost_usd = cost_usd(usage)
        # The subject facts are recorded by the code that resolved them. A
        # reader deriving the BuildMax protocol from Harbor's provider slug
        # would be re-implementing map_provider in another language, and the
        # two would drift the first time either side learned a provider.
        context.metadata = trial_metadata(envelope) | self._subject_metadata()

    def _subject_metadata(self) -> dict[str, Any]:
        """What this adapter resolved, as opposed to what the run reported."""
        access = self.model_connection
        return {
            "adapter_version": ADAPTER_VERSION,
            "artifact_digest": self._artifact_digest,
            "buildmax_provider": map_provider(access.provider),
            "harbor_provider": access.provider,
            "reasoning": self._reasoning_effort,
            # Whether the run could be priced at all, which decides how to read
            # a missing cost: unpriced, or priced and free.
            "priced": self._pricing is not None,
            "max_iterations": self._max_iterations,
            # Stated, not assumed by a later reader. The trial home carries one
            # model entry and nothing else, so there is no sandbox block and no
            # permission rule; print mode allows every tool. An unreported
            # boundary is indistinguishable from an unresolved one, and reading
            # it favourably would credit the subject with protection it never
            # had.
            "sandboxed": False,
            "permissions": "allow_all",
        }

    def _read_envelope(self) -> dict[str, Any] | None:
        path = self.logs_dir / self._RESULT_FILENAME
        try:
            body = path.read_text()
        except OSError:
            self.logger.warning("buildmax wrote no result envelope at %s", path)
            return None
        try:
            envelope = json.loads(body)
        except json.JSONDecodeError:
            self.logger.warning("buildmax result envelope at %s did not parse", path)
            return None
        if not isinstance(envelope, dict):
            return None
        return envelope

    def _run_flags(self) -> str:
        if self._max_iterations is None:
            return ""
        return f"--max-iterations {int(self._max_iterations)} "

    def _settings_yaml(self) -> str:
        if not self.model_name:
            raise ValueError("buildmax needs a model: pass -m <provider>/<model>")
        access = self.model_connection
        return render_settings(
            model=model_id(self.model_name),
            provider=map_provider(access.provider),
            base_url=access.configured_base_url or access.base_url,
            api_key=access.api_key,
            context_window=self._context_window,
            max_tokens=self._max_tokens,
            reasoning=self._reasoning_effort,
            pricing=self._pricing,
        )


__all__ = ["Buildmax"]
