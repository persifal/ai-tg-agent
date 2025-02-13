FROM debian:bullseye-slim

RUN apt-get update && \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY ai-tg-bot  /app/
COPY conf.yaml /app/

RUN chmod +x /app/ai-tg-bot
CMD ["./ai-tg-bot"]

