- Server `public_id` columns now store the 20-character canonical text form
  instead of raw bytes, so direct database queries show the same IDs the API
  does. Breaking for existing server databases: there is no migration —
  recreate the database (`./make kind down && ./make kind up`, or drop the
  Compose MySQL volume).
