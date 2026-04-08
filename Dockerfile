FROM docker.io/library/golang@sha256:03b6aaab8e80cfb1d42bc33e5190c7e78e1e6ea5a431942843f26f50ad7920a8 AS builder
WORKDIR /src

ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /conmon ./cmd/conmon

FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39
COPY --from=builder /conmon /conmon
ENTRYPOINT ["/conmon"]
