- Artifacts can now be given a revocable public link that opens without a
  BuildMax login and renders in the Portal (Markdown formatted, HTML in a
  sandbox); `UploadArtifact(share=true)` returns one, and the server builds it
  from the new `public_base_url` / `BUILDMAX_PUBLIC_BASE_URL` setting.
