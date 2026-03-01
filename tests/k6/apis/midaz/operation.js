import http from 'k6/http';
import * as env from '../../config/envConfig.js'
import * as log from '../../helper/logger.js'
import * as headers from '../../helper/headers.js';


const TRANSACTION_URL = env.data.url.transaction;

export function listByAccountId(token, organizationId, ledgerId, accountId, filter) {
    const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}/operations?${filter}`

    const requestOptions = {
        headers: headers.build(token)
    };

    const res = http.get(url, requestOptions);

    log.response(res);

    return res;
}
