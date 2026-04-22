# syntax=docker/dockerfile:1.7
#
# Build context MUST be the repo root, so the /docs markdown is included.
# From repo root:   docker build -f website/Dockerfile .

FROM node:22-alpine AS deps
WORKDIR /app/website
RUN corepack enable
COPY website/package.json website/yarn.lock* ./
RUN yarn install --frozen-lockfile --network-timeout 600000

FROM node:22-alpine AS build
WORKDIR /app/website
RUN corepack enable
COPY --from=deps /app/website/node_modules ./node_modules
COPY website/ ./
# Docs live at repo root and are globbed at build time via `base: '../docs'`
# in src/content.config.ts. They get baked into dist/, not needed at runtime.
COPY docs/ /app/docs/
RUN yarn build

FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3000

RUN addgroup -g 1001 -S app && adduser -S -u 1001 -G app app

COPY --from=build --chown=app:app /app/website/dist ./dist
COPY --from=deps  --chown=app:app /app/website/node_modules ./node_modules
COPY --from=build --chown=app:app /app/website/package.json ./package.json

USER app
EXPOSE 3000
CMD ["node", "./dist/server/entry.mjs"]
