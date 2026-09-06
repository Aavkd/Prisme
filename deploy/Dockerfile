FROM golang:1.26.8-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agent ./cmd/agent && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/probe ./cmd/probe
RUN mkdir /state && chown 10001:10001 /state

FROM scratch AS api
COPY --from=build /out/api /api
COPY --from=build /out/probe /probe
USER 10001:10001
ENTRYPOINT ["/api"]

FROM scratch AS agent
COPY --from=build /out/agent /agent
COPY --from=build --chown=10001:10001 /state /state
USER 10001:10001
ENTRYPOINT ["/agent"]
