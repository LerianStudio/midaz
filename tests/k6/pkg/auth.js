import http from 'k6/http';
import * as env from '../config/envConfig.js'
import * as log from '../helper/logger.js'
import exec from 'k6/execution';

const AUTH_URL = env.data.url.auth;

export function generateToken() {
  if (!env.AUTH_ENABLED) {
    return "";
  }

  if (!env.data.variables.client_id || !env.data.variables.client_secret) {
    exec.test.abort('Missing OAuth credentials. Set K6_CLIENT_ID and K6_CLIENT_SECRET.');
  }

  const url = `${AUTH_URL}/v1/login/oauth/access_token`;
  const payload = JSON.stringify({
    grantType: "client_credentials",
    clientId: env.data.variables.client_id,
    clientSecret: env.data.variables.client_secret
  });

  const params = {
    headers: {
      "Content-Type": "application/json"
    }
  };

  const res = http.post(url, payload, params);

  log.post("Auth", res);

  if (res.status != 200) {
    exec.test.abort("Authentication failed");
  }

  return JSON.parse(res.body).accessToken;
}
