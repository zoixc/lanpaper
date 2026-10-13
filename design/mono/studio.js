// SPDX-License-Identifier: MIT
// Runs after radical.js and before the production modules: reuse real controls.
(() => {
    document.documentElement.classList.add('studio');
    const $ = selector => document.querySelector(selector);
    $('#tplCreate').content.querySelector('.field').classList.add('studio-name-field');
    $('.catalog-eyebrow').remove();
    $('.catalog-intro').remove();
    $('.catalog-signature').remove();
    $('.catalog-label').textContent = 'КОЛЛЕКЦИЯ';
    const row = $('.appbar__row--top');
    row.prepend($('.page-head'));
    const utilities = document.createElement('div');
    utilities.className = 'studio-utilities';
    const caption = document.createElement('span');
    caption.className = 'studio-caption';
    caption.textContent = 'Ваше пространство';
    utilities.append(caption, $('#themeBtn'), $('#settingsBtn'));
    $('.catalog-rail').append(utilities);
    const rail = $('.catalog-rail');
    rail.id = 'studioNavigation';
    const toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.id = 'studioNavToggle';
    toggle.className = 'icon-btn studio-nav-toggle';
    toggle.setAttribute('aria-controls', rail.id);
    toggle.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="m14 6-6 6 6 6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
    row.prepend(toggle);
    const actionRow = $('.appbar__actions');
    const create = $('#newLinkBtn');
    const theme = $('#themeBtn');
    const settings = $('#settingsBtn');
    // Mobile actions share a real flex row, not independently positioned boxes.
    const mobileActions = document.createElement('div');
    mobileActions.className = 'studio-mobile-actions';
    mobileActions.setAttribute('role', 'group');
    mobileActions.setAttribute('aria-label', 'Действия приложения');
    rail.append(mobileActions);
    const mobileLayout = matchMedia('(max-width: 760px)');
    let collapsed = false;
    function placeActions() {
        const focused = document.activeElement;
        mobileActions.hidden = collapsed || !mobileLayout.matches;
        if (collapsed) actionRow.append(create, theme, settings);
        else if (mobileLayout.matches) mobileActions.append(theme, settings, create);
        else {
            rail.insertBefore(create, $('.catalog-label'));
            utilities.append(theme, settings);
        }
        if ([create, theme, settings].includes(focused)) focused.focus({ preventScroll: true });
    }
    mobileLayout.addEventListener('change', placeActions);
    try { collapsed = localStorage.getItem('lp-studio-nav-collapsed') === 'true'; } catch (_) {}
    function setCollapsed(value) {
        collapsed = value;
        document.documentElement.classList.toggle('sidebar-collapsed', value);
        toggle.setAttribute('aria-expanded', String(!value));
        const label = value ? 'Развернуть навигацию' : 'Свернуть навигацию';
        toggle.setAttribute('aria-label', label);
        toggle.title = label;
        // These actions must stay reachable when the rail is hidden.
        placeActions();
        try { localStorage.setItem('lp-studio-nav-collapsed', String(value)); } catch (_) {}
    }
    toggle.addEventListener('click', () => setCollapsed(!collapsed));
    setCollapsed(collapsed);
    $('.brand').href = '/admin?design=studio&sample=collection';
})();
