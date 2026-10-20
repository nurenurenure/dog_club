# syntax=docker/dockerfile:1
# Build context is the repository root: types are generated from backend/api/openapi.yaml.
FROM node:24.1.0-bookworm-slim AS deps
WORKDIR /src/frontend
# npm 11.3 bundled with this image fails to resolve vitest's optional peers.
RUN npm install -g npm@11.19.1
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund

FROM deps AS checks
COPY backend/api/openapi.yaml /src/backend/api/openapi.yaml
COPY frontend/ ./
CMD ["sh", "-ec", "npm run typecheck && npm test"]

FROM checks AS build
RUN npm run build

# Static files and a same-origin proxy to the API; runs as an unprivileged user.
FROM nginxinc/nginx-unprivileged:1.28.0-alpine AS runtime
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/frontend/dist /usr/share/nginx/html
EXPOSE 8080
