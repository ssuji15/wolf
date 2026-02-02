FROM --platform=$BUILDPLATFORM golang:1.24 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o worker ./cmd/wolf_worker

FROM ubuntu:22.04

ARG TARGETPLATFORM
ARG TARGETARCH

WORKDIR /usr/local/bin

COPY --from=builder /app/worker .

RUN apt-get update \
    && apt-get install -y clang \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY ./cmd/wolf_worker/stdc++.h /usr/include/c++/12/bits/stdc++.h

RUN clang++ -O1  -fno-exceptions -fno-rtti -x c++-header /usr/include/c++/12/bits/stdc++.h -o /usr/include/c++/12/bits/stdc++.h.pch
#RUN clang++ -O0 -x c++-header -fno-exceptions -fno-rtti /usr/include/c++/12/bits/stdc++.h -o /usr/include/c++/12/bits/stdc++.h.pch

RUN useradd -u 1000 -m -s /usr/sbin/nologin appuser

RUN chown appuser:appuser /usr/local/bin/worker \
    && chmod 0755 /usr/local/bin/worker

USER 1000

ENTRYPOINT ["/usr/local/bin/worker"]
