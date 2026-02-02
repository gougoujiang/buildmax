# Load .env (KEY=value, one per line; lines starting with # are skipped).
# .env is read from the same directory as this script (project root).
# In terminal: cd to project root, then: source loadenv.sh
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.env"
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck source=/dev/null
  . "$ENV_FILE"
  set +a
fi
