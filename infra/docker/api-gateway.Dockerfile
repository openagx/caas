FROM node:22-alpine AS builder
WORKDIR /app
COPY services/api-gateway/package.json services/api-gateway/pnpm-lock.yaml* ./
COPY pnpm-workspace.yaml ./
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm install --frozen-lockfile || pnpm install
COPY services/api-gateway/ ./
COPY proto/ /app/proto/
RUN pnpm build

FROM node:22-alpine
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY --from=builder /app/package.json ./
COPY --from=builder /app/node_modules/ ./node_modules/
COPY --from=builder /app/dist/ ./dist/
COPY proto/ /app/proto/
EXPOSE 3001
CMD ["node", "dist/index.js"]
