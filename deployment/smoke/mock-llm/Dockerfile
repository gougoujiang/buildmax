FROM golang:1.26-alpine3.23 AS build
WORKDIR /src
# The server is a packaging of internal/testsupport/mockllm, so the build needs
# the module rather than one file. Only the package it imports is copied, which
# keeps the layer small and the build free of downloads: mockllm is stdlib-only.
COPY go.mod go.sum ./
COPY internal/testsupport/mockllm ./internal/testsupport/mockllm
COPY deployment/smoke/mock-llm ./deployment/smoke/mock-llm
RUN CGO_ENABLED=0 go build -o /mock-llm ./deployment/smoke/mock-llm

FROM scratch
COPY --from=build /mock-llm /mock-llm
EXPOSE 8080
ENTRYPOINT ["/mock-llm"]
