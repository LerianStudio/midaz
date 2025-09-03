FROM node:22-alpine

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

EXPOSE 8081

ENTRYPOINT ["/scripts/entrypoint.sh"]

CMD ["npm", "run", "start"]