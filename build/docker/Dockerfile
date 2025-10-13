FROM alpine:3.20
RUN apk add --no-cache ca-certificates

COPY jc2aws /jc2aws
RUN chmod +x /usr/bin/jc2aws

WORKDIR /work
ENTRYPOINT ["/jc2aws"]
