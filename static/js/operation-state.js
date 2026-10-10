/* SPDX-License-Identifier: MIT */

export function createOperationState() {
    const active = new Set();
    return Object.freeze({
        begin(key) {
            if (active.has(key)) return false;
            active.add(key);
            return true;
        },
        end(key) { active.delete(key); },
        has(key) { return active.has(key); },
        size() { return active.size; }
    });
}

export function observeConnectivity(target, callback) {
    const update = () => callback(target.navigator.onLine !== false);
    target.addEventListener('online', update);
    target.addEventListener('offline', update);
    update();
    return () => {
        target.removeEventListener('online', update);
        target.removeEventListener('offline', update);
    };
}
