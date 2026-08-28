- `./make build cli <os/arch>` cross-builds a static CLI for another platform
  and names the artifact `buildmax-<os>-<arch>`, leaving the host binary in
  place. It is how the CLI gets into a container image the project does not
  own, such as an external benchmark's.
