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
    /* Коробка для уменьшенной «Новая ссылка»: она лежит в шапке всегда, и
       потому её появление можно анимировать (ширина 0 ↔ 32px). Саму кнопку
       анимировать нельзя — она одна на обеих сторонах и переезжает по DOM. */
    const createSlot = document.createElement('span');
    createSlot.className = 'appbar__create';
    actionRow.prepend(createSlot);
    const mobile = matchMedia('(max-width: 760px)');
    let collapsed = false;
    let returnCreate = 0;
    try { collapsed = localStorage.getItem('lp-studio-nav-collapsed') === 'true'; } catch (_) {}
    /* На телефоне рельса — не колонка, а горизонтальная полоса, и в ней лежат
       и фильтры, и единственный экземпляр кнопок: .sidebar-collapsed снимает
       её целиком (display: none), так что «свернуть» здесь значит спрятать
       органы управления, не оставив им другого места. Свёрнутое состояние
       поэтому учитывается только там, где рельса — боковая колонка, а
       переключатель на телефоне не предлагается вовсе. */
    const collapsible = () => !mobile.matches;
    const folded = () => collapsible() && collapsed;
    function labels() {
        const text = t(folded() ? 'expand_navigation' : 'collapse_navigation');
        toggle.setAttribute('aria-label', text);
        toggle.title = text;
    }
    /* У кнопок один дом: на широком экране это шапка справа — и при свёрнутой,
       и при раскрытой рельсе, — а на телефоне своя строка в полосе рельсы,
       которая теперь не сворачивается. Раньше тема и настройки жили в низу
       рельсы и при каждом сворачивании перелетали в шапку: один и тот же
       узел переезжал, дублировать его нельзя (id один, фокус один). */
    /* Длительность берётся из CSS, чтобы число жило в одном месте. */
    function slideDuration() {
        const raw = getComputedStyle(document.documentElement)
            .getPropertyValue('--dur-slow')
            .trim();
        const value = parseFloat(raw) || 0;
        return raw.indexOf('ms') > -1 ? value : value * 1000;
    }
    function placeActions() {
        const focused = document.activeElement;
        clearTimeout(returnCreate);
        if (mobile.matches) {
            createSlot.classList.remove('appbar__create--leaving');
            mobileActions.append(theme, settings, create);
        } else {
            actionRow.append(theme, settings);
            /* «Новая ссылка» живёт в рельсе, а свёрнутая рельса снимается
               целиком — кнопка переезжает в шапку и возвращается обратно.
               Обратно она уезжает не сразу, а когда коробка схлопнулась:
               иначе кнопка исчезает рывком, а поиск прыгает на её место.
               Пока коробка схлопывается, на кнопке держится компактный вид,
               чтобы в узкой коробке не мелькнула полная подпись. */
            if (folded()) {
                createSlot.classList.remove('appbar__create--leaving');
                createSlot.append(create);
            } else if (createSlot.contains(create)) {
                createSlot.classList.add('appbar__create--leaving');
                returnCreate = setTimeout(function () {
                    const keepFocus = document.activeElement === create;
                    rail.insertBefore(create, label);
                    createSlot.classList.remove('appbar__create--leaving');
                    if (keepFocus) create.focus({ preventScroll: true });
                }, slideDuration());
            } else {
                rail.insertBefore(create, label);
            }
        }
        mobileActions.hidden = !mobile.matches;
        if ([create, theme, settings].includes(focused)) focused.focus({ preventScroll: true });
    }
    function apply() {
        document.documentElement.classList.toggle('sidebar-collapsed', folded());
        toggle.hidden = !collapsible();
        toggle.setAttribute('aria-expanded', String(!folded()));
        labels();
        placeActions();
    }
    mobile.addEventListener('change', apply);
    toggle.addEventListener('click', () => {
        collapsed = !collapsed;
        apply();
        try { localStorage.setItem('lp-studio-nav-collapsed', String(collapsed)); } catch (_) {}
    });
    apply();
    return { labels };
}
