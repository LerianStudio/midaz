import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const ONBOARDING_URL = env.data.url.onboarding;

export function create(token, organizationId, payload) {
    const url = `${ONBOARDING_URL}/v1/organizations/${organizationId}/ledgers`;

    const requestOptions = {
        headers: headers.build(token)
    };

    const res = http.post(url, payload, requestOptions);

    log.response(res);

    return res;
}

export function list(token, organizationId) {
    const url = `${ONBOARDING_URL}/v1/organizations/${organizationId}/ledgers?sort_order=desc`;

    const requestOptions = {
        headers: headers.build(token)
    };

    const res = http.get(url, requestOptions);

    log.response(res);

    return res;
}
