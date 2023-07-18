FROM golang:1.15.6@sha256:9a3218c894d35d855c819fb6ec52333920fea69f90a793b6570620a128643363 AS builder

WORKDIR /go/src/github.com/alibaba/open-local
COPY . .
RUN make build && chmod +x bin/open-local

FROM alpine:3.9@sha256:65b3a80ebe7471beecbc090c5b2cdd0aafeaefa0715f8f12e40dc918a3a70e32
LABEL maintainers="Alibaba Cloud Authors"
LABEL description="open-local is a local disk management system"
RUN apk update && apk upgrade && apk add util-linux coreutils e2fsprogs e2fsprogs-extra xfsprogs xfsprogs-extra blkid file open-iscsi jq
COPY --from=builder /go/src/github.com/alibaba/open-local/bin/open-local /bin/open-local
COPY --from=thebeatles1994/open-local:tools /usr/local/bin/restic-amd64 /usr/local/bin/restic
ENTRYPOINT ["open-local"]