FROM alpine@sha256:72c42ed48c3a2db31b7dafe17d275b634664a708d901ec9fd57b1529280f01fb

ADD secrets-injector /secrets-injector
ENTRYPOINT ["./secrets-injector"]
