// SPDX-License-Identifier: MIT
(() => {
    const current = new URL(location.href);
    if (current.searchParams.get('embedded') === '1') document.documentElement.classList.add('demo-embedded');
    document.querySelectorAll('[data-demo], [data-sample]').forEach(link => {
        const next = new URL(current);
        if (link.dataset.demo) {
            next.searchParams.set('design', link.dataset.demo);
            link.setAttribute('aria-current', (current.searchParams.get('design') || 'new') === link.dataset.demo ? 'page' : 'false');
        } else {
            next.searchParams.set('sample', link.dataset.sample);
            link.setAttribute('aria-current', (current.searchParams.get('sample') || 'single') === link.dataset.sample ? 'page' : 'false');
        }
        link.href = next.pathname + next.search;
    });
})();
