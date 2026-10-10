/* SPDX-License-Identifier: MIT */

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function createOverlayController(options) {
    const stack = [];
    const doc = options.document;

    const current = () => stack.length ? stack[stack.length - 1] : null;
    const notify = () => {
        const active = current();
        options.onChange(active, stack.length, stack.slice());
        if (options.announce) options.announce(active ? (active.label || '') : '');
    };

    function remove(entry, keepFocus, dismiss) {
        entry.node.remove();
        entry.scrim.remove();
        if (dismiss && entry.onDismiss) entry.onDismiss();
        if (!keepFocus && entry.origin && entry.origin.isConnected) {
            entry.origin.focus({ preventScroll: true });
        }
    }

    function close(keepFocus) {
        const entry = stack.pop();
        if (!entry) return;
        remove(entry, keepFocus, true);
        const parent = current();
        if (parent) {
            parent.node.removeAttribute('aria-hidden');
            parent.node.inert = false;
        }
        notify();
        return entry;
    }

    function closeAll(keepFocus) {
        while (stack.length) remove(stack.pop(), true, true);
        options.onChange(null, 0, []);
        if (!keepFocus && options.restoreTarget && options.restoreTarget.isConnected) {
            options.restoreTarget.focus({ preventScroll: true });
        }
    }

    function open(node, opts = {}) {
        const origin = doc.activeElement && doc.activeElement !== doc.body ? doc.activeElement : null;
        if (!opts.stack) closeAll(true);
        const parent = current();
        if (parent) {
            parent.node.setAttribute('aria-hidden', 'true');
            parent.node.inert = true;
        }
        const scrim = options.createScrim(() => { if (!opts.persistent) close(); });
        const labelledBy = node.getAttribute('aria-labelledby');
        const heading = labelledBy && doc.getElementById ? doc.getElementById(labelledBy) : null;
        const entry = { node, scrim, origin, kind: opts.kind || 'other', persistent: !!opts.persistent,
            onDismiss: opts.onDismiss || null,
            label: opts.label || node.getAttribute('aria-label') || (heading && heading.textContent) || '' };
        stack.push(entry);
        options.host().append(scrim, node);
        notify();
        const target = opts.focus || node.querySelector('[autofocus], input, button, [tabindex]:not([tabindex="-1"])');
        setTimeout(() => target && target.focus({ preventScroll: true }), 40);
        return node;
    }

    function keydown(event) {
        const active = current();
        if (!active) return false;
        if (event.key === 'Escape') {
            if (!active.persistent) close();
            event.preventDefault();
            return true;
        }
        if (event.key !== 'Tab') return false;
        const focusables = Array.from(active.node.querySelectorAll(FOCUSABLE))
            .filter(element => element.offsetParent !== null && !element.inert);
        if (!focusables.length) { event.preventDefault(); active.node.focus(); return true; }
        const first = focusables[0];
        const last = focusables[focusables.length - 1];
        if (event.shiftKey && doc.activeElement === first) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && doc.activeElement === last) { event.preventDefault(); first.focus(); }
        return true;
    }

    /* A virtual keyboard changes the visual viewport without a window resize.
       Keep the focused control visible instead of moving or resizing the sheet. */
    if (options.visualViewport) {
        options.visualViewport.addEventListener('resize', () => {
            if (current() && doc.activeElement && current().node.contains(doc.activeElement)) {
                doc.activeElement.scrollIntoView({ block: 'nearest', inline: 'nearest' });
            }
        });
    }

    return Object.freeze({ open, close, closeAll, current, keydown, depth: () => stack.length });
}
