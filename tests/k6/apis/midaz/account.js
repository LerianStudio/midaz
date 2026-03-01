import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const ONBOARDING_URL = env.data.url.onboarding;

export function create(token, organizationId, ledgerId, payload) {
    const url = `${ONBOARDING_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts`;

    const requestOptions = {
        headers: headers.build(token)
    };

    const res = http.post(url, payload, requestOptions);
    log.response(res);

    return res;
}

export function getByAlias(token, organizationId, ledgerId, alias) {
    const url = `${ONBOARDING_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/alias/${alias}`;

    const requestOptions = {
        headers: headers.build(token)
    };

    const res = http.get(url, requestOptions);

    log.response(res);

    return res;
}

export function remove(token, organizationId, ledgerId, accountId) {
    const url = `${ONBOARDING_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`;

    const requestOptions = {
        headers: headers.build(token)
    };

    const res = http.del(url, null, requestOptions);

    log.response(res);

    return res;
}
