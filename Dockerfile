FROM alpine:latest
COPY output/namespace-termination-locker /
ENTRYPOINT [ "./namespace-termination-locker" ]