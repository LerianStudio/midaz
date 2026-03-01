import * as midaz from '../pkg/midaz.js'
import exec from 'k6/execution';

export function createOrganization(token) {

    const payload = JSON.stringify({
        "legalName": "Lerian Studio",
        "doingBusinessAs": "Lerian",
        "legalDocument": "123456789",
        "status": {
            "code": "ACTIVE",
            "description": "Organization Created"
        },
        "address": {
            "line1": "Avenida Paulista, 1234 - Centro",
            "line2": "CJ 203",
            "zipCode": "04696040",
            "city": "Sao Paulo",
            "state": "SP",
            "country": "BR"
        },
        "metadata": {
            "email": "contact@lerian.studio",
            "phone": "+5511999990000"
        }
    });

    const res = midaz.organization.create(token, payload);

    if (res.status !== 201 && res.status !== 200) {
        console.error(`[ERROR] Failed to create organization: ${res.status} - ${res.body}`);
        exec.test.abort(`Failed to create organization: ${res.status}`);
    }

    const body = JSON.parse(res.body);
    if (!body || !body.id) {
        console.error(`[ERROR] Invalid response body: ${res.body}`);
        exec.test.abort('Invalid response: missing organization id');
    }

    return body.id;
}

export function createLedger(token, organizationId) {
    const payload = JSON.stringify({
        "name": "Lerian Ledger",
        "status": {
            "code": "ACTIVE",
            "description": "Ledger created"
        }
    });
    const res = midaz.ledger.create(token, organizationId, payload);

    if (res.status !== 201 && res.status !== 200) {
        console.error(`[ERROR] Failed to create ledger: ${res.status} - ${res.body}`);
        exec.test.abort(`Failed to create ledger: ${res.status}`);
    }

    const body = JSON.parse(res.body);
    if (!body || !body.id) {
        console.error(`[ERROR] Invalid response body: ${res.body}`);
        exec.test.abort('Invalid response: missing ledger id');
    }

    return body.id;
}

export function createAsset(token, organizationId, ledgerId, type, code) {
    const payload = JSON.stringify({
        "name": `${code} Asset`,
        "type": type,
        "code": code,
        "status": {
            "code": "ACTIVE",
            "description": "Created"
        }
    });

    const res = midaz.asset.create(token, organizationId, ledgerId, payload);

    if (res.status !== 201 && res.status !== 200) {
        console.error(`[ERROR] Failed to create asset: ${res.status} - ${res.body}`);
        exec.test.abort(`Failed to create asset: ${res.status}`);
    }

    const body = JSON.parse(res.body);
    if (!body || !body.id) {
        console.error(`[ERROR] Invalid response body: ${res.body}`);
        exec.test.abort('Invalid response: missing asset id');
    }

    return body.id;
}

export function createAccounts(token, organizationId, ledgerId, assetCode) {
    const accounts = ["@account1_BRL", "@account2_BRL", "@account3_BRL", "@account4_BRL", "@account5_BRL", "@account6_BRL", "@account7_BRL", "@account8_BRL", "@account9_BRL", "@account10_BRL"];

    for (const accountAlias of accounts) {
        const payload = JSON.stringify({
            "assetCode": assetCode,
            "name": `${assetCode} Account`,
            "alias": accountAlias,
            "type": "deposit",
            "allowSending": true,
            "allowReceiving": true,
            "status": {
                "code": "ACTIVE",
                "description": "Account Created"
            }
        });
        const res = midaz.account.create(token, organizationId, ledgerId, payload);
    }
}

export function createAssetAccounts(token, organizationId, ledgerId, quantity, assetCode, accountPrefix) {
    console.log(`[START] Creating ${quantity} ${assetCode} accounts`);

    for (var account = 1; account <= quantity; account++) {
        const payload = JSON.stringify({
            "assetCode": assetCode,
            "name": `${assetCode} Account`,
            "alias": `${accountPrefix}${account}_${assetCode}`,
            "type": "deposit",
            "allowSending": true,
            "allowReceiving": true,
            "status": {
                "code": "ACTIVE",
                "description": "Account Created"
            }
        });
        const res = midaz.account.create(token, organizationId, ledgerId, payload);
    }
    console.log(`[END] ${quantity} ${assetCode} accounts created`);
}

export function createSegment(token, organizationId, ledgerId) {
    const payload = JSON.stringify({
        "name": "K6 Fee Segment",
        "status": {
            "code": "ACTIVE",
            "description": "Segment Created"
        }
    });
    const res = midaz.segment.create(token, organizationId, ledgerId, payload);

    if (res.status !== 201 && res.status !== 200) {
        console.error(`[ERROR] Failed to create segment: ${res.status} - ${res.body}`);
        exec.test.abort(`Failed to create segment: ${res.status}`);
    }

    const body = JSON.parse(res.body);
    if (!body || !body.id) {
        console.error(`[ERROR] Invalid response body: ${res.body}`);
        exec.test.abort('Invalid response: missing segment id');
    }

    return body.id;
}

export function getLastOrganization(token) {
    const res = midaz.organization.list(token);

    if (res.status !== 200) {
        console.error(`[ERROR] Failed to list organizations: ${res.status} - ${res.body}`);
        exec.test.abort(`Failed to list organizations: ${res.status}`);
    }

    const body = JSON.parse(res.body);
    if (!body || !body.items || body.items.length === 0) {
        console.error(`[ERROR] No organizations found: ${res.body}`);
        exec.test.abort('No organizations found');
    }

    return body.items[0].id;
}

export function getLastLedger(token, organizationId) {
    const res = midaz.ledger.list(token, organizationId);

    if (res.status !== 200) {
        console.error(`[ERROR] Failed to list ledgers: ${res.status} - ${res.body}`);
        exec.test.abort(`Failed to list ledgers: ${res.status}`);
    }

    const body = JSON.parse(res.body);
    if (!body || !body.items || body.items.length === 0) {
        console.error(`[ERROR] No ledgers found: ${res.body}`);
        exec.test.abort('No ledgers found');
    }

    return body.items[0].id;
}
