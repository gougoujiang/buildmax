- `buildmax admin` gained `model list`, `model add`, `model enable`, and `model
  disable`: managing the deployment's model catalog over the authenticated Admin
  API, the automation peer of the Portal Models area. `model add` sends the
  provider key in the request body only; it is stored encrypted and never read
  back, and a deployment with no encryption key configured refuses a model that
  carries one.
