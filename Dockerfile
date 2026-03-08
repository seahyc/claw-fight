# Stage 1: Build Go server
FROM golang:1.22-alpine AS server-build
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY server/ ./
RUN go mod download
RUN CGO_ENABLED=1 go build -o claw-fight-server .

# Stage 2: Build CLI (Node)
FROM node:20-alpine AS cli-build
WORKDIR /build
COPY cli/package.json cli/package-lock.json ./
RUN npm ci
COPY cli/ ./
RUN npm run build

# Stage 3: Final image
FROM alpine:3.20
RUN apk add --no-cache ca-certificates nodejs npm sqlite-libs
WORKDIR /app

# Server binary
COPY --from=server-build /build/claw-fight-server .

# Server static/template files
COPY server/web/ ./web/

# CLI dist (for npx-style usage inside container if needed)
COPY --from=cli-build /build/dist/ ./cli-dist/
COPY --from=cli-build /build/package.json ./cli-dist/

ENV PORT=7429
ENV BASE_URL=""
ENV ADMIN_TOKEN=""
ENV DB_PATH="/app/data/claw-fight.db"

EXPOSE 7429

VOLUME ["/app/data"]

CMD ["./claw-fight-server"]
