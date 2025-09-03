# ==================================================
# OPTIMIZED MULTI-STAGE DOCKERFILE FOR MIDAZ CONSOLE
# Reduces image size from ~5GB to ~200-300MB
# ==================================================

# Stage 1: Dependencies installation
FROM node:22-alpine AS dependencies

WORKDIR /usr/src/app

# Install system dependencies needed for node-gyp and native modules
RUN apk add --no-cache \
    libc6-compat \
    python3 \
    make \
    g++ \
    git \
    bash

# Copy package files
COPY components/console/package*.json ./

# Install production dependencies only, with optimizations
ARG GITHUB_TOKEN
RUN if [ -n "${GITHUB_TOKEN}" ]; then \
        echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc; \
    fi && \
    npm ci --only=production --no-audit --no-fund --ignore-scripts && \
    npm cache clean --force && \
    rm -f ~/.npmrc

# Stage 2: Build stage  
FROM node:22-alpine AS builder

WORKDIR /usr/src/app

# Install system dependencies
RUN apk add --no-cache bash git python3 make g++

# Copy production dependencies from previous stage
COPY --from=dependencies /usr/src/app/node_modules ./node_modules

# Copy package files and install all dependencies (including dev)
COPY components/console/package*.json ./

ARG GITHUB_TOKEN
RUN if [ -n "${GITHUB_TOKEN}" ]; then \
        echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc; \
    fi && \
    npm install --no-audit --no-fund && \
    rm -f ~/.npmrc

# Copy source code and build configuration
COPY components/console/ .

# Build the application
RUN npm run set-env && \
    npm run build && \
    npm cache clean --force

# Stage 3: Production runtime (minimal)
FROM node:22-alpine AS runtime

WORKDIR /usr/src/app

# Install only essential runtime dependencies
RUN apk add --no-cache \
    bash \
    tini \
    dumb-init \
    gettext \
    && rm -rf /var/cache/apk/* \
    && rm -rf /usr/share/man/* \
    && rm -rf /tmp/* \
    && rm -rf /var/tmp/*

# Create non-root user for security
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# Set production environment variables
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=8081

# Copy only the standalone build and necessary files
COPY --from=builder --chown=nextjs:nodejs /usr/src/app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /usr/src/app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /usr/src/app/public ./public
COPY --from=builder --chown=nextjs:nodejs /usr/src/app/.env ./.env

# Ensure the nextjs user can modify the .env file
RUN chown nextjs:nodejs .env && chmod 644 .env

# Copy the entrypoint script
COPY --from=builder --chown=nextjs:nodejs /usr/src/app/scripts/entrypoint.sh /scripts/entrypoint.sh
RUN chmod +x /scripts/entrypoint.sh

# Ensure public directory exists and has proper permissions
RUN mkdir -p public && chown -R nextjs:nodejs public

# Switch to non-root user
USER nextjs

# Expose the port
EXPOSE 8081

# Add health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/health || exit 1

# Use dumb-init as PID 1 for proper signal handling
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/scripts/entrypoint.sh"]

# Start the standalone Next.js server
CMD ["node", "server.js"]