# --- Dependencies Stage --------------------------
FROM node:22-alpine AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app

# Copy package files for better caching
COPY components/console/package.json components/console/package-lock.json* ./

# Install dependencies with authentication and optimizations
RUN if [ -f package-lock.json ]; then \
      echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc && \
      npm ci --only=production --ignore-scripts && \
      rm -f ~/.npmrc; \
    else \
      echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc && \
      npm install --only=production --ignore-scripts && \
      rm -f ~/.npmrc; \
    fi

# --- Builder Stage ------------------------------
FROM node:22-alpine AS builder
WORKDIR /app

# Copy dependencies from deps stage
COPY --from=deps /app/node_modules ./node_modules

# Copy package files
COPY components/console/package.json components/console/package-lock.json* ./

# Install all dependencies (including dev dependencies for build)
RUN if [ -f package-lock.json ]; then \
      echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc && \
      npm ci && \
      rm -f ~/.npmrc; \
    else \
      echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc && \
      npm install && \
      rm -f ~/.npmrc; \
    fi

# Copy necessary files for build
COPY components/console/.env.example .env.example
COPY components/console/scripts/entrypoint.sh /scripts/entrypoint.sh
RUN chmod +x /scripts/entrypoint.sh && npm run set-env

# Copy source code (use .dockerignore to exclude unnecessary files)
COPY components/console/ .

# Build application with optimizations
ENV NEXT_TELEMETRY_DISABLED=1
ENV NODE_ENV=production
RUN npm run build && npm prune --production

# --- Runner Stage ---
FROM node:22-alpine AS runner

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create app directory and non-root user
WORKDIR /app
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# Set environment variables
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=8081

# Copy built application with correct ownership
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/front-config.yml ./front-config.yml
COPY --from=builder --chown=nextjs:nodejs /scripts/entrypoint.sh /scripts/entrypoint.sh

# Switch to non-root user
USER nextjs

# Expose port
EXPOSE 8081

# Add healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/admin/health/alive || exit 1

# Use dumb-init and entrypoint
ENTRYPOINT ["dumb-init", "/scripts/entrypoint.sh"]
CMD ["node", "server.js"]