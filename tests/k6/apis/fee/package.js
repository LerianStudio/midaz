import http from 'k6/http';
import * as env from '../../config/envConfig.js'
import * as log from '../../helper/logger.js'
import * as headers from '../../helper/headers.js';

const FEE_URL = env.data.url.fee;

export function create(token, organizationId, payload) {
    const url = `${FEE_URL}/v1/packages`

    const requestOptions = {
        headers: headers.build(token, {
            'X-Organization-Id': organizationId
        })
    };

    const res = http.post(url, payload, requestOptions);

    log.post("Plugin Fee - Package", res);

    return res;
}
