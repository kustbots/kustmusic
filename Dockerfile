# Build the Go program with the native voice-call library, then run it next to
# ffmpeg, yt-dlp and a JavaScript runtime (yt-dlp needs one for YouTube).
FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ ca-certificates curl unzip zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# NTgCalls, the native library that joins voice chats. It is fetched at build
# time so it never has to be committed.
RUN curl -sL "https://github.com/pytgcalls/ntgcalls/releases/download/v2.2.5/ntgcalls.linux-x86_64-static_libs.zip" -o /tmp/ntgcalls.zip \
    && unzip -o /tmp/ntgcalls.zip -d /tmp/ntgcalls \
    && mkdir -p internal/core/vc/ntgcalls/include internal/core/vc/ntgcalls/lib \
    && cp /tmp/ntgcalls/include/ntgcalls.h internal/core/vc/ntgcalls/include/ntgcalls.h \
    && cp /tmp/ntgcalls/lib/libntgcalls.a internal/core/vc/ntgcalls/lib/libntgcalls.a \
    && rm -rf /tmp/ntgcalls /tmp/ntgcalls.zip

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o /app/bin/kustmusic .

FROM python:3.12-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg ca-certificates curl unzip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://deno.land/install.sh | DENO_INSTALL=/usr/local sh \
    && pip install --no-cache-dir "yt-dlp[default]"

COPY --from=builder /app/bin/kustmusic /usr/local/bin/kustmusic

ENV PORT=8000
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/kustmusic"]
