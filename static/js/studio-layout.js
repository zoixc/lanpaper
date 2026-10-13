/* SPDX-License-Identifier: MIT */

// Reuse real controls/IDs and handlers. Only presentation preferences are stored.
export function initializeStudio(t) {
    if (document.documentElement.dataset.layout !== 'studio') return null;
    const $ = selector => document.querySelector(selector);
    document.documentElement.classList.add('studio', 'radical');
    const rail = document.createElement('aside');
    rail.className = 'catalog-rail';
    rail.id = 'studioNavigation';
    rail.setAttribute('data-i18n-aria', 'navigation');
    const label = document.createElement('p');
    label.className = 'catalog-label';
    label.dataset.i18n = 'collection';
    const create = $('#newLinkBtn');
    const theme = $('#themeBtn');
    const settings = $('#settingsBtn');
    rail.append($('.brand'), create, label, $('.tools'));
    $('.shell').prepend(rail);
    const row = $('.appbar__row--top');
    row.prepend($('.page-head'), $('.search'));
    $('.appbar__row--tools').remove();
    const utilities = document.createElement('div');
    utilities.className = 'studio-utilities';
    utilities.append(theme, settings);
    rail.append(utilities);
    const mobileActions = document.createElement('div');
    mobileActions.className = 'studio-mobile-actions';
    mobileActions.setAttribute('role', 'group');
    mobileActions.setAttribute('data-i18n-aria', 'app_actions');
    rail.append(mobileActions);
    const toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.id = 'studioNavToggle';
    toggle.className = 'icon-btn studio-nav-toggle';
    toggle.setAttribute('aria-controls', rail.id);
    toggle.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="m14 6-6 6 6 6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
    row.prepend(toggle);
    const actionRow = $('.appbar__actions');
    const mobile = matchMedia('(max-width: 760px)');
    let collapsed = false;
    try { collapsed = localStorage.getItem('lp-studio-nav-collapsed') === 'true'; } catch (_) {}
    function labels() {
        const text = t(collapsed ? 'expand_navigation' : 'collapse_navigation');
        toggle.setAttribute('aria-label', text);
        toggle.title = text;
    }
    function placeActions() {
        const focused = document.activeElement;
        mobileActions.hidden = collapsed || !mobile.matches;
        if (collapsed) actionRow.append(create, theme, settings);
        else if (mobile.matches) mobileActions.append(theme, settings, create);
        else { rail.insertBefore(create, label); utilities.append(theme, settings); }
        if ([create, theme, settings].includes(focused)) focused.focus({ preventScroll: true });
    }
    function apply() {
        document.documentElement.classList.toggle('sidebar-collapsed', collapsed);
        toggle.setAttribute('aria-expanded', String(!collapsed));
        labels();
        placeActions();
    }
    mobile.addEventListener('change', placeActions);
    toggle.addEventListener('click', () => {
        collapsed = !collapsed;
        apply();
        try { localStorage.setItem('lp-studio-nav-collapsed', String(collapsed)); } catch (_) {}
    });
    apply();
    return { labels };
}
