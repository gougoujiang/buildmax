- The published container images now apply their base image's pending security
  updates at build time. A base tag lags its branch's updates, so
  `ghcr.io/gougoujiang/buildmax:0.2.0-alpha.3` and the matching
  `buildmax-portal` image shipped openssl 3.5.7-r0 while alpine had already
  published 3.5.8-r0, and the release scan failed on CVE-2026-14456 after both
  images were pushed. The binaries in the archives were never affected: they
  are built with `CGO_ENABLED=0` and do not link the system openssl.
