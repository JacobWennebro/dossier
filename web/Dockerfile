# Build from the repository root:
#   docker build -f web/Dockerfile -t dossier-web .
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X dossier/internal/cli.version=${VERSION}" \
      -o /out/dossier-web ./web

# distroless/static ships CA certificates (needed for RDAP and homepage
# requests) and runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dossier-web /dossier-web
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/dossier-web"]
