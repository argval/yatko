ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm as builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
# Static binary (no cgo): it needs no libc, so the runtime image can be tiny.
RUN CGO_ENABLED=0 go build -v -trimpath -ldflags="-s -w" -o /run-app .


# A small image boots faster on a cold container. distroless/static carries CA
# certificates, tzdata and /tmp, but no shell; the web server runs no external
# commands (only the separate cmd/artifactscan worker shells out).
FROM gcr.io/distroless/static-debian12

COPY --from=builder /run-app /usr/local/bin/
CMD ["run-app"]
