FROM node:22-alpine AS builder

WORKDIR /usr/src/app

COPY components/console/package*.json ./

RUN echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}" > ~/.npmrc && \
    apk update && apk add bash && \
    npm install && \
    npm cache clean --force && \
    rm -f ~/.npmrc

COPY components/console/scripts/entrypoint.sh /scripts/entrypoint.sh

RUN chmod +x /scripts/entrypoint.sh

COPY components/console/.env.example .env.example

RUN npm run set-env

COPY components/console/ .

RUN npm run build

# --- Runner Stage ----

FROM node:22-alpine AS runner

WORKDIR /usr/src/app
ENV PORT=8081

COPY --from=builder /usr/src/app/.next/standalone ./
COPY --from=builder /usr/src/app/.next/static ./.next/static
COPY --from=builder /usr/src/app/public ./public
COPY --from=builder /usr/src/app/front-config.yml ./front-config.yml
COPY --from=builder /usr/src/app/scripts/entrypoint.sh /scripts/entrypoint.sh

EXPOSE 8081

ENTRYPOINT ["/scripts/entrypoint.sh"]

CMD ["node", "server.js"]