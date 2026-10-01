# Builds Ben Olmstead's reference Malbolge interpreter (public domain) with the
# minimal harness patches in harness.c, then runs every *.mb in /work:
#   /work/X.mb  [+ X.in, X.steps]  ->  X.refout (stdout), X.referr (status line), X.refrc
FROM alpine:3.22
RUN apk add --no-cache gcc musl-dev
COPY harness.c /src/harness.c
RUN gcc -O2 -w -o /usr/local/bin/malbolge-ref /src/harness.c
COPY run.sh /usr/local/bin/run.sh
ENTRYPOINT ["/bin/sh", "/usr/local/bin/run.sh"]
