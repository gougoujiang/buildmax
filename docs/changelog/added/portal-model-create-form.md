- The Portal admin Models area now has an "Add a model" form, so a System
  Administrator can add a managed model without the server command line. The API
  key is a password field, sent only in the request body, stored encrypted, and
  never shown again; it is cleared as soon as the model is added. A deployment
  with no encryption key configured reports that it cannot accept a credential.
