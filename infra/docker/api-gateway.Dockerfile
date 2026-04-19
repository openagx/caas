FROM node:22-alpine AS deps
WORKDIR /app
COPY services/api-gateway/package.json ./services/api-gateway/package.json
COPY pnpm-lock.yaml ./pnpm-lock.yaml
COPY pnpm-workspace.yaml ./pnpm-workspace.yaml
RUN corepack enable && corepack prepare pnpm@10.18.1 --activate
RUN pnpm install --frozen-lockfile --filter ./services/api-gateway...

FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/services/api-gateway/package.json ./services/api-gateway/package.json
COPY services/api-gateway/ ./services/api-gateway/
COPY proto/ ./proto/
WORKDIR /app/services/api-gateway
RUN pnpm build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup -S caas && adduser -S caas -G caas
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/services/api-gateway/package.json ./services/api-gateway/package.json
COPY --from=builder /app/services/api-gateway/dist ./services/api-gateway/dist
COPY proto/ /app/proto/
WORKDIR /app/services/api-gateway
USER caas
EXPOSE 3001
HEALTHCHECK --interval=15s --timeout=3s --retries=5 CMD wget -qO- http://localhost:3001/health || exit 1
CMD ["node", "dist/index.js"]
