import * as env from '../config/envConfig.js'
import exec from 'k6/execution';

function maskSensitive(value) {
    return String(value)
        .replace(/\b\d{11}\b/g, '***CPF***')
        .replace(/\b\d{14}\b/g, '***CNPJ***')
        .replace(/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g, '***EMAIL***');
}

function summarizeBody(body) {
    if (!body || body.length === 0) {
        return 'empty-body';
    }

    if (env.LOG_FULL_BODY) {
        return maskSensitive(body).slice(0, 600);
    }

    try {
        const parsed = JSON.parse(body);
        const parts = [];
        if (parsed.code !== undefined) parts.push(`code=${parsed.code}`);
        if (parsed.message !== undefined) parts.push(`message=${maskSensitive(parsed.message)}`);
        if (parsed.status !== undefined) parts.push(`status=${parsed.status}`);

        return parts.length > 0 ? parts.join(' | ') : 'json-body';
    } catch (_) {
        return 'non-json-body';
    }
}

export function post(Service, Response) {
    if(env.LOG == "DEBUG") {
        console.log(`POST ${Service} | ${Response.status_text} | ${Response.timings.duration} ms`)
    }
    else if (env.LOG == "ERROR" && Response.status != 201){
        let message = summarizeBody(Response.body);
        try {
            const parsed = JSON.parse(Response.body);
            if (parsed.message !== undefined) {
                message = maskSensitive(parsed.message);
            }
        } catch (_) {
            // keep summarized message
        }
        console.log(`POST ${Service} | ${Response.status_text} | ${message} | ${Response.timings.duration} ms`)
    }
}

export function response(Response) {
    if (Response.status >= 400 && (env.LOG == "DEBUG" || env.LOG == "ERROR")) {
        console.log(`${Response.request.method} | ${Response.status_text} | ${summarizeBody(Response.body)} | ${Response.request.url}`)
        if (env.ABORT_ON_ERROR) {
            exec.test.abort('Execution aborted after HTTP error');
        }
    }
}
