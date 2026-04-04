# Build stage
FROM node:20-alpine AS builder

WORKDIR /app/ui

COPY ui/package.json ./
RUN npm install

COPY ui .
RUN npm run build

# Production stage
FROM nginx:alpine

COPY --from=builder /app/ui/dist /usr/share/nginx/html
COPY deployments/docker/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 3000

CMD ["nginx", "-g", "daemon off;"]
