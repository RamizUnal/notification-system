# syntax=docker/dockerfile:1.7

# ---- build: type-check and bundle the console ----
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY . .
ARG VITE_JAEGER_URL=http://localhost:16686
ENV VITE_JAEGER_URL=$VITE_JAEGER_URL
RUN npm run build

# ---- serve: static files plus a same-origin proxy to the API ----
FROM nginx:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD wget -qO- http://127.0.0.1/healthz >/dev/null || exit 1
