#!/bin/sh
# Write the Portal's runtime configuration before nginx starts.
#
# nginx:alpine runs every executable in /docker-entrypoint.d/ at container
# start. This one replaces the empty config.js shipped in the image with the
# deployment's API URL, so the same published image serves any server.
#
#   BUILDMAX_API_BASE   URL of buildmax-server, e.g. https://api.example.com
#                       "/" means the Portal's own origin, for a reverse proxy
#                       that fronts both. Unset leaves the bundle's built-in
#                       default (http://localhost:5678).
#
# Note for operators: unless the API is same-origin, the server's cors_origin
# must name the Portal's origin, or the browser blocks every request.
set -eu

config_file=/usr/share/nginx/html/config.js
api_base=${BUILDMAX_API_BASE:-}

# JSON string escaping for a value that reaches the browser as JavaScript.
escaped=$(printf '%s' "$api_base" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')

cat >"$config_file" <<EOF
window.__BUILDMAX_CONFIG__ = { apiBase: "$escaped" };
EOF

if [ -n "$api_base" ]; then
    echo "buildmax-portal: API base set to $api_base"
else
    echo "buildmax-portal: BUILDMAX_API_BASE is unset; using the bundle default"
fi
