FROM golang:1.18 AS builder

WORKDIR /go/src/github.com/apecloud/open-local
COPY . .
RUN /bin/sh -c 'pwd && make build && chmod +x bin/open-local'

FROM sting2me/open-local-base:latest
LABEL maintainers="Alibaba Cloud Authors"
LABEL description="open-local is a local disk management system"
COPY --from=builder /go/src/github.com/apecloud/open-local/bin/open-local /bin/open-local
ENTRYPOINT ["open-local"]
