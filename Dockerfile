FROM golang:1.15.6 AS builder

WORKDIR /go/src/github.com/alibaba/open-local
COPY . .
RUN make build && chmod +x bin/open-local

FROM alpine:3.6
LABEL maintainers="Alibaba Cloud Authors"
LABEL description="open-local is a local disk management system"
RUN apk update && apk upgrade && apk add util-linux coreutils xfsprogs xfsprogs-extra e2fsprogs blkid open-iscsi
COPY --from=builder /go/src/github.com/alibaba/open-local/bin/open-local /bin/open-local
ENTRYPOINT ["open-local"]