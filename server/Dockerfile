FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -g 10001 linha && adduser -D -u 10001 -G linha linha
COPY --chmod=755 bin/linha-server /usr/local/bin/linha-server
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/linha-server"]
