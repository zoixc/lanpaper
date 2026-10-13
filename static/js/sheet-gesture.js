/* SPDX-License-Identifier: MIT */

// Only the grip owns touch gestures. The scrollable body and browser gestures
// are left alone. No document listeners, move loop, animation frames or timers.
export function attachSheetGesture(grip, options) {
    let start = null;
    let suppressClick = false;
    const eligible = () => options.canDismiss();
    function down(event) {
        if (!event.isPrimary || !['touch', 'pen'].includes(event.pointerType) || !eligible()) return;
        suppressClick = false;
        start = { id: event.pointerId, x: event.clientX, y: event.clientY, time: event.timeStamp };
        if (grip.setPointerCapture) grip.setPointerCapture(event.pointerId);
    }
    function up(event) {
        if (!start || event.pointerId !== start.id) return;
        const dx = Math.abs(event.clientX - start.x);
        const dy = event.clientY - start.y;
        const elapsed = event.timeStamp - start.time;
        start = null;
        if (dy >= 64 && dy > dx * 1.5 && elapsed < 1200 && eligible()) {
            suppressClick = true;
            event.preventDefault();
            options.dismiss();
        }
    }
    function cancel() { start = null; }
    function click(event) {
        if (suppressClick) { suppressClick = false; event.preventDefault(); return; }
        if (eligible()) options.dismiss();
    }
    const handlers = { pointerdown: down, pointerup: up, pointercancel: cancel, lostpointercapture: cancel, click };
    for (const [type, fn] of Object.entries(handlers)) grip.addEventListener(type, fn);
    return () => {
        start = null;
        for (const [type, fn] of Object.entries(handlers)) grip.removeEventListener(type, fn);
    };
}
