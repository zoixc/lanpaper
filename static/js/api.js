// SPDX-License-Identifier: MIT

export class ApiError extends Error {
    constructor(message, options = {}) {
        super(message, options.cause ? { cause: options.cause } : undefined);
        this.name = 'ApiError';
        this.kind = options.kind || 'http';
        this.status = options.status || 0;
        this.code = options.code || (this.status ? `http_${this.status}` : this.kind);
        this.retryable = options.retryable === true;
    }
}

export async function request(url, options = {}) {
    const method = options.method || 'GET';
    const isForm = options.isForm === true;
    const init = {
        method,
        credentials: 'same-origin',
        signal: options.signal,
        headers: isForm ? undefined : { 'Content-Type': 'application/json' }
    };
    if (options.body !== undefined && options.body !== null) {
        init.body = isForm ? options.body : JSON.stringify(options.body);
    }

    let response;
    try {
        response = await fetch(url, init);
    } catch (cause) {
        if (cause && cause.name === 'AbortError') {
            throw new ApiError('Request cancelled', { kind: 'cancelled', code: 'cancelled', cause });
        }
        throw new ApiError('Network request failed', { kind: 'network', code: 'network_error', retryable: true, cause });
    }

    if (response.status === 401) {
        throw new ApiError('Signed out', { kind: 'authentication', code: 'session_expired', status: 401 });
    }
    if (!response.ok) {
        const text = (await response.text().catch(() => '')).trim();
        const code = response.headers.get('x-error-code') || `http_${response.status}`;
        throw new ApiError(text || `HTTP ${response.status}`, {
            status: response.status,
            code,
            retryable: response.status === 408 || response.status === 429 || response.status >= 500
        });
    }
    if (response.status === 204) return null;
    const type = response.headers.get('content-type') || '';
    return type.includes('application/json') ? response.json() : null;
}
