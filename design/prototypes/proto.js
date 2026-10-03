/* SPDX-License-Identifier: MIT */
/* ============================================================
   LANPAPER PROTOTYPES — общее поведение
   Ничего не знает о бэкенде: все действия подтверждаются тостом.
   ============================================================ */
(function () {
    'use strict';

    /* ---------- Тема ---------- */
    const store = {
        get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
        set(k, v) { try { localStorage.setItem(k, v); } catch (e) {} },
    };

    const root = document.documentElement;
    const savedTheme = store.get('lp-theme');
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    setTheme(savedTheme || (prefersDark ? 'dark' : 'light'));

    function setTheme(t) {
        root.dataset.theme = t;
        store.set('lp-theme', t);
        document.querySelectorAll('[data-theme-toggle]').forEach((btn) => {
            btn.setAttribute('aria-pressed', String(t === 'dark'));
            btn.setAttribute('aria-label', t === 'dark' ? 'Светлая тема' : 'Тёмная тема');
        });
    }

    document.addEventListener('click', (e) => {
        const t = e.target.closest('[data-theme-toggle]');
        if (t) { setTheme(root.dataset.theme === 'dark' ? 'light' : 'dark'); }
    });

    /* ---------- Меню ---------- */
    function closeMenus(except) {
        document.querySelectorAll('.menu-wrap.open').forEach((w) => {
            if (w !== except) {
                w.classList.remove('open');
                const b = w.querySelector('[aria-expanded]');
                if (b) b.setAttribute('aria-expanded', 'false');
            }
        });
    }

    document.addEventListener('click', (e) => {
        const toggle = e.target.closest('[data-menu-toggle], .menu-wrap > [aria-haspopup]');
        if (toggle) {
            e.preventDefault();
            const wrap = toggle.closest('.menu-wrap');
            const willOpen = !wrap.classList.contains('open');
            closeMenus(wrap);
            wrap.classList.toggle('open', willOpen);
            toggle.setAttribute('aria-expanded', String(willOpen));
            if (willOpen) flipMenu(wrap);
            return;
        }
        if (!e.target.closest('.menu')) closeMenus();

        const item = e.target.closest('.menu__item[data-toast]');
        if (item) {
            toast(item.dataset.toast, item.dataset.toastType || 'accent');
            closeMenus();
        }
    });

    // Меню у нижней кромки раскрывается вверх.
    function flipMenu(wrap) {
        const menu = wrap.querySelector('.menu');
        if (!menu || window.innerWidth <= 768) return;
        menu.classList.remove('menu--up');
        const r = menu.getBoundingClientRect();
        if (r.bottom > window.innerHeight - 8) menu.classList.add('menu--up');
    }

    /* ---------- Тосты ---------- */
    function toast(text, type) {
        let host = document.querySelector('.toasts');
        if (!host) {
            host = document.createElement('div');
            host.className = 'toasts';
            document.body.appendChild(host);
        }
        const icons = {
            success: '<polyline points="20 6 9 17 4 12"/>',
            danger: '<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>',
            accent: '<polyline points="20 6 9 17 4 12"/>',
        };
        const el = document.createElement('div');
        el.className = 'toast toast--' + (type || 'accent');
        el.setAttribute('role', 'status');
        el.innerHTML =
            '<span class="toast__icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
            'stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">' +
            (icons[type] || icons.accent) + '</svg></span><span></span>';
        el.lastElementChild.textContent = text;
        host.appendChild(el);
        setTimeout(() => {
            el.classList.add('is-hiding');
            setTimeout(() => el.remove(), 220);
        }, 2600);
    }
    window.protoToast = toast;

    /* ---------- Копирование ---------- */
    document.addEventListener('click', (e) => {
        const btn = e.target.closest('[data-copy]');
        if (!btn) return;
        const value = btn.dataset.copy;
        const done = () => toast('URL скопирован', 'success');
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(value).then(done, done);
        } else { done(); }
    });

    /* ---------- Шторка / панель ---------- */
    const drawer = document.querySelector('.drawer');
    const scrim = document.querySelector('.scrim');

    function openDrawer(name) {
        if (!drawer) return;
        if (name) {
            const t = drawer.querySelector('[data-drawer-title]');
            const s = drawer.querySelector('[data-drawer-sub]');
            if (t && name.title) t.textContent = name.title;
            if (s && name.sub) s.textContent = name.sub;
        }
        drawer.classList.add('open');
        if (scrim) scrim.classList.add('open');
        document.body.style.overflow = 'hidden';
    }
    function closeDrawer() {
        if (!drawer) return;
        drawer.classList.remove('open');
        if (scrim) scrim.classList.remove('open');
        document.body.style.overflow = '';
    }
    window.protoDrawer = { open: openDrawer, close: closeDrawer };

    document.addEventListener('click', (e) => {
        const opener = e.target.closest('[data-drawer-open]');
        if (opener) {
            openDrawer({ title: opener.dataset.title, sub: opener.dataset.sub });
            return;
        }
        if (e.target.closest('[data-drawer-close]') || e.target === scrim) closeDrawer();
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') { closeMenus(); closeDrawer(); }
    });

    /* ---------- Табы ---------- */
    document.addEventListener('click', (e) => {
        const tab = e.target.closest('[role="tab"]');
        if (!tab) return;
        const group = tab.closest('[data-tabs]');
        if (!group) return;
        group.querySelectorAll('[role="tab"]').forEach((t) => {
            t.setAttribute('aria-selected', String(t === tab));
        });
        // Панели лежат рядом с группой табов, а не внутри неё.
        const scope = group.parentElement || document;
        scope.querySelectorAll('[data-tabpanel]').forEach((p) => {
            p.hidden = p.dataset.tabpanel !== tab.dataset.tab;
        });
    });

    /* ---------- Поиск ---------- */
    const search = document.querySelector('[data-search-input]');
    if (search) {
        const field = search.closest('.field');
        const sync = () => {
            if (field) field.classList.toggle('has-value', search.value.length > 0);
        };
        search.addEventListener('input', () => { sync(); applyFilters(); });
        sync();
        const clear = document.querySelector('[data-search-clear]');
        if (clear) clear.addEventListener('click', () => { search.value = ''; sync(); applyFilters(); search.focus(); });
    }

    /* ---------- Фильтры-чипсы ---------- */
    document.querySelectorAll('[data-filter]').forEach((chip) => {
        chip.addEventListener('click', () => {
            const isOnly = chip.dataset.filterKind === 'single';
            if (isOnly) {
                document.querySelectorAll('[data-filter-kind="single"]').forEach((c) => {
                    c.setAttribute('aria-pressed', String(c === chip));
                });
            } else {
                chip.setAttribute('aria-pressed', String(chip.getAttribute('aria-pressed') !== 'true'));
            }
            applyFilters();
        });
    });

    function applyFilters() {
        const items = Array.from(document.querySelectorAll('[data-item]'));
        if (!items.length) return;
        const q = (search ? search.value : '').trim().toLowerCase();

        const active = Array.from(document.querySelectorAll('[data-filter][aria-pressed="true"]'))
            .map((c) => c.dataset.filter)
            .filter((f) => f && f !== 'all');

        items.forEach((item) => {
            const hay = (item.dataset.search || '').toLowerCase();
            const matchQ = !q || hay.includes(q);
            const matchF = active.every((f) => (item.dataset.tags || '').split(' ').includes(f));
            item.classList.toggle('u-hide', !(matchQ && matchF));
        });

        // Пересчёт счётчиков у чипсов
        document.querySelectorAll('[data-filter]').forEach((chip) => {
            const f = chip.dataset.filter;
            const n = chip.querySelector('.chip__count');
            if (!n || !f) return;
            const count = items.filter((item) => {
                const hay = (item.dataset.search || '').toLowerCase();
                const matchQ = !q || hay.includes(q);
                const matchF = f === 'all' || (item.dataset.tags || '').split(' ').includes(f);
                return matchQ && matchF;
            }).length;
            n.textContent = String(count);
        });

        const counter = document.querySelector('[data-result-count]');
        const shown = items.filter((i) => !i.classList.contains('u-hide')).length;
        if (counter) counter.textContent = String(shown);
        const emptyState = document.querySelector('[data-empty-state]');
        if (emptyState) emptyState.hidden = shown !== 0;
        const listEmpty = document.querySelector('[data-empty-list]');
        if (listEmpty) listEmpty.hidden = shown === 0;
    }
    window.protoApplyFilters = applyFilters;

    /* ---------- Сортировка ---------- */
    document.querySelectorAll('[data-sort-option]').forEach((opt) => {
        opt.addEventListener('click', () => {
            const list = document.querySelector('[data-list]');
            if (!list) return;
            const key = opt.dataset.sortOption;
            const dir = opt.dataset.sortDir || 'asc';
            const items = Array.from(list.querySelectorAll('[data-item]'));
            items.sort((a, b) => {
                const av = (a.dataset[key] || '').toString();
                const bv = (b.dataset[key] || '').toString();
                const an = parseFloat(av), bn = parseFloat(bv);
                const bothNum = !isNaN(an) && !isNaN(bn);
                let cmp = bothNum ? an - bn : av.localeCompare(bv, 'ru');
                return dir === 'desc' ? -cmp : cmp;
            });
            items.forEach((i) => list.appendChild(i));
            const label = document.querySelector('[data-sort-label]');
            if (label) label.textContent = opt.dataset.sortLabel || opt.textContent.trim();
            list.querySelectorAll('[data-sort-option]') && closeMenus();
            toast('Сортировка: ' + (opt.dataset.sortLabel || opt.textContent.trim()), 'accent');
        });
    });

    /* ---------- Вид: сетка / список ---------- */
    document.querySelectorAll('[data-view-btn]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const view = btn.dataset.viewBtn;
            document.querySelectorAll('[data-view-btn]').forEach((b) => {
                b.setAttribute('aria-pressed', String(b === btn));
            });
            const list = document.querySelector('[data-list]');
            if (list) list.dataset.view = view;
            store.set('lp-view', view);
        });
    });
    const savedView = store.get('lp-view');
    if (savedView) {
        const btn = document.querySelector('[data-view-btn="' + savedView + '"]');
        if (btn) btn.click();
    }

    /* ---------- Режим выбора ---------- */
    let selectMode = false;

    function syncSelection() {
        const boxes = Array.from(document.querySelectorAll('[data-item] [data-select] input'));
        const checked = boxes.filter((b) => b.checked);
        const bar = document.querySelector('[data-selection-bar]');
        if (bar) {
            bar.hidden = !selectMode;
            const n = bar.querySelector('[data-selected-count]');
            if (n) n.textContent = String(checked.length);
            const all = bar.querySelector('[data-select-all]');
            if (all) all.checked = boxes.length > 0 && checked.length === boxes.length;
            const allInd = bar.querySelector('[data-select-all-indeterminate]');
            if (allInd) allInd.checked = checked.length > 0 && checked.length < boxes.length;
        }
        const list = document.querySelector('[data-list]');
        if (list) list.classList.toggle('is-selecting', selectMode);
        document.querySelectorAll('[data-item]').forEach((item) => {
            const box = item.querySelector('[data-select]');
            if (!box) return;
            const boxed = box.querySelector('input');
            item.classList.toggle('is-selected', !!(boxed && boxed.checked));
        });
        document.querySelectorAll('[data-action-count]').forEach((el) => {
            el.textContent = String(checked.length);
        });
        document.querySelectorAll('[data-requires-selection]').forEach((el) => {
            el.toggleAttribute('disabled', checked.length === 0);
        });
    }

    document.addEventListener('click', (e) => {
        if (e.target.closest('[data-select-mode]')) {
            selectMode = !selectMode;
            if (!selectMode) {
                document.querySelectorAll('[data-item] [data-select] input').forEach((b) => { b.checked = false; });
            }
            syncSelection();
            return;
        }
        if (e.target.closest('[data-select-cancel]')) {
            selectMode = false;
            document.querySelectorAll('[data-item] [data-select] input').forEach((b) => { b.checked = false; });
            syncSelection();
            return;
        }
        if (e.target.closest('[data-select-all-wrap]') || e.target.closest('label[data-select-all-label]')) {
            // обрабатывается change-событием чекбокса
        }
        const action = e.target.closest('[data-bulk-action]');
        if (action) {
            const n = document.querySelectorAll('[data-item] [data-select] input:checked').length;
            toast(action.dataset.bulkAction + ': ' + n + ' ' + plural(n), 'accent');
        }
    });

    document.addEventListener('change', (e) => {
        const all = e.target.closest('[data-select-all]');
        if (all) {
            document.querySelectorAll('[data-item] [data-select] input').forEach((b) => { b.checked = all.checked; });
            syncSelection();
            return;
        }
        if (e.target.closest('[data-select]')) syncSelection();
    });

    // Клик по карточке в режиме выбора — тоже выбор
    document.addEventListener('click', (e) => {
        if (!selectMode) return;
        const item = e.target.closest('[data-item]');
        if (!item) return;
        if (e.target.closest('button, a, input, label, .menu-wrap')) return;
        const box = item.querySelector('[data-select] input');
        if (box) { box.checked = !box.checked; syncSelection(); }
    });

    /* ---------- Демо-действия ---------- */
    document.addEventListener('click', (e) => {
        const el = e.target.closest('[data-demo]');
        if (!el) return;
        toast(el.dataset.demo, el.dataset.demoType || 'accent');
    });

    /* ---------- Демо-загрузка файла ---------- */
    document.querySelectorAll('[data-file-demo]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = 'image/*,video/mp4,video/webm';
            input.addEventListener('change', () => {
                if (input.files && input.files[0]) {
                    toast('Загружено: ' + input.files[0].name, 'success');
                }
            });
            input.click();
        });
    });

    /* ---------- Горячие клавиши (как в приложении) ---------- */
    document.addEventListener('keydown', (e) => {
        if (e.target.matches('input, textarea, select')) return;
        if (!(e.ctrlKey || e.metaKey)) return;
        const k = e.key.toLowerCase();
        if (k === 'f' || k === 'а') {
            e.preventDefault();
            if (search) search.focus();
        } else if (k === 'g' || k === 'п') {
            e.preventDefault();
            const btns = document.querySelectorAll('[data-view-btn]');
            if (btns.length === 2) (btns[0].getAttribute('aria-pressed') === 'true' ? btns[1] : btns[0]).click();
        }
    });
    document.addEventListener('keydown', (e) => {
        if (e.target.matches('input, textarea, select')) return;
        if (e.key.toLowerCase() === 't' && !e.ctrlKey && !e.metaKey) {
            setTheme(root.dataset.theme === 'dark' ? 'light' : 'dark');
        }
    });

    /* ---------- Старт ---------- */
    applyFilters();
    syncSelection();
})();

function plural(n) {
    const m10 = n % 10, m100 = n % 100;
    if (m10 === 1 && m100 !== 11) return 'ссылка';
    if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return 'ссылки';
    return 'ссылок';
}
