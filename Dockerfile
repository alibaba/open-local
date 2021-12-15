FROM golang:1.15.6 AS builder

WORKDIR /go/src/github.com/alibaba/open-local
COPY . .
RUN make build && chmod +x bin/open-local

FROM centos:7 AS centos
RUN yum install -y file xfsprogs e4fsprogs lvm2 util-linux
COPY --from=builder /go/src/github.com/alibaba/open-local/bin/open-local /bin/open-local
ENTRYPOINT ["open-local"]