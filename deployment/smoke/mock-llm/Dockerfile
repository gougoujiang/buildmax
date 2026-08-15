FROM golang:1.26-alpine3.23 AS build
WORKDIR /src
COPY deployment/smoke/mock-llm/main.go .
RUN CGO_ENABLED=0 go build -o /mock-llm main.go

FROM scratch
COPY --from=build /mock-llm /mock-llm
EXPOSE 8080
ENTRYPOINT ["/mock-llm"]
