FROM golang:1.11.2-alpine3.8 AS build

# Install git and dep
# Run `docker build --no-cache .` to update dependencies
RUN apk add --no-cache git
RUN go get github.com/golang/dep/cmd/dep

# List qac dependencies with Gopkg.toml and Gopkg.lock
# These layers are only re-built when Gopkg files are updated
COPY Gopkg.lock Gopkg.toml /go/src/enforcer/
WORKDIR /go/src/enforcer

# Install our app library dependencies
RUN dep ensure -vendor-only

# Copy the source code and build it
COPY ./cmd/pod-resources-enforcer/ /go/src/enforcer/
RUN go build -o /bin/enforcer

# This results in a single layer image
FROM alpine:3.8
RUN adduser -S -D -H -h /app appuser
USER appuser
COPY --from=build /bin/enforcer /bin/enforcer
ENTRYPOINT ["/bin/enforcer"]