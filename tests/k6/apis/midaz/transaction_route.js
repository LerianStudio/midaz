import http from 'k6/http';
import * as env from '../../config/envConfig.js'
import * as log from '../../helper/logger.js'
import * as headers from '../../helper/headers.js';

const TRANSACTION_URL = env.data.url.transaction;

export function create(token, organizationId, ledgerId, payload) {
    const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes`

    const requestOptions = {
        headers: headers.build(token)
    };
    const res = http.post(url, payload, requestOptions);

    log.response(res);

    return res;
}
