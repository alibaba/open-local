FROM golang:1.15.6 AS builder

WORKDIR /go/src/github.com/oecp/open-local-storage-service
COPY . .
RUN make build

FROM centos:7 AS centos
RUN yum install -y file xfsprogs e4fsprogs lvm2 util-linux
COPY --from=builder /go/src/github.com/oecp/open-local-storage-service/bin/open-local /bin/open-local
RUN chmod +x /bin/open-local
ENTRYPOINT ["open-local"]