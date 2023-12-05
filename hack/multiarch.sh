# create linux arm64 and amd64 environment named mybuilder
docker buildx create --name mybuilder --platform linux/arm64,linux/amd64 --use
mybuilder
# open-local-base image build multiarch image with mybuilder
docker buildx build --platform=linux/arm64/v8,linux/amd64 --builder mybuilder --push  --tag openlocal/open-local-base:latest  -f Dockerfile.base .
# open-local-tools image build multiarch image with mybuilder
docker buildx build --platform=linux/arm64/v8,linux/amd64 --builder mybuilder --push  --tag openlocal/open-local-tools:latest  -f Dockerfile.tools .

# open-local main image

make image && make image-arm64
# tag=v0.8.0-alpha
docker tag openlocal/open-local:$tag openlocal/open-local:$tag-amd
docker push openlocal/open-local:$tag-amd
docker tag openlocal/open-local:$tag-arm64 openlocal/open-local:$tag-arm64
docker push openlocal/open-local:$tag-arm64
docker manifest create openlocal/open-local:$tag --amend openlocal/open-local:$tag-arm64 --amend openlocal/open-local:$tag-amd
docker manifest push openlocal/open-local:$tag



