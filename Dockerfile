# syntax=docker/dockerfile:1

# Build stage: compile a static binary so the runtime image can be distroless.
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
# No third-party dependencies (stdlib only), so there is nothing to download; copying
# go.mod alone keeps this layer cached until the module changes.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/reportingd ./cmd/reportingd
# Create the storage dir here (distroless has no shell to mkdir/chown) so it can be copied
# into the runtime image owned by the non-root user (uid 65532).
RUN mkdir -p /data/reports

# Runtime stage: distroless static, non-root.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/reportingd /reportingd
# Screenshots + issue metadata persist on this volume so the reconciler survives restarts.
# Owned by the nonroot user so the service can write to it (and an attached volume inherits it).
COPY --from=build --chown=65532:65532 /data/reports /data/reports
VOLUME ["/data/reports"]
EXPOSE 8080
ENV REPORTING_ADDR=":8080" \
    REPORTING_STORAGE_DIR="/data/reports"
USER nonroot:nonroot
ENTRYPOINT ["/reportingd"]
