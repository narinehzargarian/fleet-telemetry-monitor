# One image, three binaries (server, processor, simulator). Pick one with the container command.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# Distroless: no shell or package manager, runs as non-root. Small and low attack surface.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
USER nonroot:nonroot
EXPOSE 8080 9091
ENTRYPOINT ["/usr/local/bin/server"]
