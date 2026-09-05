- The local kind deployment now runs the worker control channel over HTTPS with
  a generated certificate, and `./make kind` verifies the worker API boundary in
  the same smoke: a labelled worker pod reaches the internal listener, an
  unlabelled pod is denied it by the NetworkPolicy, and `/api/worker` answers
  `404` on the public Service.
