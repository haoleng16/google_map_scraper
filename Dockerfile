# Build stage for Playwright dependencies
FROM golang:1.26.2-trixie AS playwright-deps
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/browsers
#ENV PLAYWRIGHT_DRIVER_PATH=/opt/

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl wget \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && go install github.com/playwright-community/playwright-go/cmd/playwright@latest \
    && mkdir -p /opt/browsers \
    && playwright install chromium --with-deps

# Build stage
FROM golang:1.26.2-trixie AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY third_party/scrapemate/go.mod third_party/scrapemate/go.sum ./third_party/scrapemate/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /usr/bin/google-maps-scraper

# Final stage
FROM debian:trixie-slim
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/browsers
ENV PLAYWRIGHT_DRIVER_PATH=/opt
WORKDIR /app

# Install only the necessary dependencies in a single layer
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libnss3 \
    libnspr4 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libdbus-1-3 \
    libxkbcommon0 \
    libatspi2.0-0 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libpango-1.0-0 \
    libcairo2 \
    libasound2 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=playwright-deps /opt/browsers /opt/browsers
COPY --from=playwright-deps /root/.cache/ms-playwright-go /opt/ms-playwright-go

RUN chmod -R 755 /opt/browsers \
    && chmod -R 755 /opt/ms-playwright-go

COPY --from=builder /usr/bin/google-maps-scraper /usr/bin/
COPY --from=builder /app/geodata/cities.db /app/geodata/cities.db

ENTRYPOINT ["google-maps-scraper"]
