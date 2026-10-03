#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Собирает final2.html из final.html (генератор, не страница).

Порядок важен: сначала карточки и футеры, потом подписи на превью.

1. карточка получает постоянный футер [Изменить ▾] [Пульт] [⋯] вместо
   hover-полосы быстрых действий, а ⋯-меню переезжает в футер;
2. добавляются карточки под разные типы медиа: вертикальный постер,
   панорама, PNG с прозрачностью, квадратный WebP, GIF, вертикальное видео;
3. на превью появляются подписи формата, пропорций, длительности и счётчика;
4. в сайдбаре — переключатель из четырёх палитр (запоминается в браузере);
5. клик по карточке открывает пульт ссылки.

Запуск:  python3 design/prototypes/_build-final2.py
"""

import os
import re

BASE = os.path.dirname(os.path.abspath(__file__)) + '/'
s = open(BASE + 'final.html', encoding='utf-8').read()


# ------------------------------------------------------------ утилиты
def sub(old, new, count=1, label=''):
    global s
    assert old in s, 'не найдено' + (f' ({label})' if label else '') + ': ' + old[:110]
    s = s.replace(old, new, count)


def cut_span(text, marker):
    """Вырезает <span ...>...</span> с учётом вложенности → (до, span, после)."""
    start = text.index(marker)
    i, depth = start, 0
    while i < len(text):
        if text.startswith('<span', i):
            depth += 1
        elif text.startswith('</span>', i):
            depth -= 1
            if depth == 0:
                end = i + len('</span>')
                return text[:start], text[start:end], text[end:]
        i += 1
    raise AssertionError('не закрыт span: ' + marker)


def cut_div(text, marker):
    start = text.index(marker)
    i, depth = start, 0
    while i < len(text):
        if text.startswith('<div', i):
            depth += 1
        elif text.startswith('</div>', i):
            depth -= 1
            if depth == 0:
                end = i + len('</div>')
                return text[:start], text[start:end], text[end:]
        i += 1
    raise AssertionError('не закрыт div: ' + marker)


def inner(html):
    """Внутренности <tag ...>…</tag>."""
    return html[html.index('>') + 1:html.rindex('</')]


def card_block(name):
    m = re.search(r'<article class="link" data-item[^>]*data-name="' + name + r'".*?</article>', s, re.S)
    assert m, 'карточка ' + name + ' не найдена'
    return m.group(0)


def card_names():
    return re.findall(r'<article class="link" data-item[^>]*data-name="([^"]+)"', s)


# ------------------------------------------------------------ иконки
ICONS = {
    'upload': '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/>',
    'pick': '<path d="M15 7a4 4 0 1 1-3 6.9L9 17H6v3H3v-3l6-6"/>',
    'caret': '<polyline points="6 9 12 15 18 9"/>',
    'sliders': '<line x1="4" y1="8" x2="20" y2="8"/><line x1="4" y1="16" x2="20" y2="16"/><circle cx="9" cy="8" r="2.2"/><circle cx="15" cy="16" r="2.2"/>',
    'link': '<path d="M10 13a5 5 0 0 0 7.5.5l3-3A5 5 0 0 0 13.1 3.1l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3A5 5 0 0 0 10.9 20.9l1.7-1.7"/>',
    'gallery': '<rect x="3" y="4" width="18" height="6" rx="2"/><rect x="3" y="14" width="18" height="6" rx="2"/><line x1="7" y1="7" x2="7.01" y2="7"/><line x1="7" y1="17" x2="7.01" y2="17"/>',
    'plus': '<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>',
    'open': '<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/>',
    'copy': '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    'pin': '<path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"/>',
    'trash': '<polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6M14 11v6"/>',
    'dots': '<circle cx="5" cy="12" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="19" cy="12" r="1.7"/>',
    'eye': '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="2.6"/>',
    'play': '<path d="M6 4l14 8-14 8z"/>',
    'image': '<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8.5" cy="9" r="1.5"/><path d="M21 15l-5-5L5 21"/>',
}


def ic(name, w=1.8, size=None, cls=''):
    style = f' style="width:{size}px;height:{size}px"' if size else ''
    klass = f' class="{cls}"' if cls else ''
    if name in ('dots', 'play'):
        return f'<svg aria-hidden="true"{klass} viewBox="0 0 24 24" fill="currentColor"{style}>{ICONS[name]}</svg>'
    return (f'<svg aria-hidden="true"{klass} viewBox="0 0 24 24" fill="none" stroke="currentColor" '
            f'stroke-width="{w}" stroke-linecap="round" stroke-linejoin="round"{style}>{ICONS[name]}</svg>')


# ============================================================
# 1. Стили, палитра, заголовок
# ============================================================
sub('<html lang="ru" data-theme="light">',
    '<html lang="ru" data-theme="light" data-palette="indigo">')
sub('<link rel="stylesheet" href="final.css">',
    '''<link rel="stylesheet" href="final.css">
<link rel="stylesheet" href="palettes.css?v=7">
<link rel="stylesheet" href="final2.css?v=7">''')
sub('<title>Lanpaper — финальный вариант (A + C + D)</title>',
    '<title>Lanpaper — финальная версия: компактно, с пультом и палитрами</title>')

# ============================================================
# 2. Переключатель палитры в сайдбаре
# ============================================================
sub('  <div class="fside__actions">',
    '''  <!-- Палитра: четыре варианта, выбор запоминается -->
  <div class="fside__palette">
    <div class="palette-label">Палитра</div>
    <div class="palette-row" role="group" aria-label="Палитра приложения">
      <button class="palette-dot" data-palette-btn="indigo" aria-pressed="true" title="Индиго" aria-label="Индиго"></button>
      <button class="palette-dot" data-palette-btn="terra" aria-pressed="false" title="Терракота" aria-label="Терракота"></button>
      <button class="palette-dot" data-palette-btn="sage" aria-pressed="false" title="Шалфей" aria-label="Шалфей"></button>
      <button class="palette-dot" data-palette-btn="graphite" aria-pressed="false" title="Графит" aria-label="Графит"></button>
    </div>
  </div>

  <div class="fside__actions">''')

# ============================================================
# 3. Футер карточки: [Изменить ▾] [Пульт] [⋯]
# ============================================================
FOOT_TPL = '''        <div class="link__foot link__foot--split">
          <div class="menu-wrap menu-wrap--grow">
            <button class="btn btn--{style} btn--sm" aria-haspopup="true" aria-expanded="false">
              {icon}
              <span class="btn__label">{label}</span>
              {caret}
            </button>
            <div class="menu menu--left">
              <div class="menu__title">{menu_title}</div>
{add_items}            </div>
          </div>
          <button class="btn btn--soft btn--sm btn--pult" data-drawer-open data-title="{title}" data-sub="{url}">
            {sliders}
            <span class="btn__label">Пульт</span>
          </button>
          <div class="menu-wrap">
            <button class="iconbtn iconbtn--sm" aria-haspopup="true" aria-expanded="false" aria-label="Ещё действия">{dots}</button>
            <div class="menu">
{more_menu}            </div>
          </div>
        </div>
'''

ADD_MEDIA = '\n'.join([
    '              <button class="menu__item" data-file-demo>' + ic('upload') + 'Файл с устройства</button>',
    '              <button class="menu__item" data-demo="Вставьте ссылку на файл">' + ic('link') + 'По ссылке (URL)</button>',
    '              <button class="menu__item" data-demo="Галерея сервера">' + ic('gallery') + 'Из галереи сервера</button>',
]) + '\n'


def menu_item(icon, label, attrs, key=''):
    tail = f'<span class="menu__key">{key}</span>' if key else ''
    return f'              <button class="menu__item" {attrs}>{ic(icon)}{label}{tail}</button>'


def more_items(title, url, empty=False, versions=''):
    out = []
    if empty:
        out.append(menu_item('upload', 'Загрузить файл', 'data-file-demo'))
    out.append(menu_item('sliders', 'Открыть пульт',
                         f'data-drawer-open data-title="{title}" data-sub="{url}"', key=versions))
    if not empty:
        out.append(menu_item('open', 'Открыть', 'data-demo="Открыто в новой вкладке"'))
    out.append(menu_item('copy', 'Копировать URL', f'data-copy="{url}"'))
    out.append('              <div class="menu__sep"></div>')
    if not empty:
        out.append(menu_item('upload', 'Заменить файл', 'data-file-demo'))
        out.append(menu_item('plus', 'Добавить в плейлист', 'data-file-demo'))
        out.append(menu_item('pin', 'Закрепить вверху', 'data-demo="Ссылка закреплена"'))
        out.append(menu_item('sliders', 'Уровень доступа', 'data-demo="Доступ: локальная сеть"'))
    else:
        out.append(menu_item('sliders', 'Уровень доступа', 'data-demo="Доступ: локальная сеть"'))
    out.append('              <div class="menu__sep"></div>')
    out.append(menu_item('trash', 'Удалить', 'data-demo="Открыто подтверждение удаления" data-demo-type="danger"'))
    return '\n'.join(out) + '\n'


def build_card(block):
    """Убирает hover-полосу, переносит ⋯-меню в футер, добавляет футер."""
    name = re.search(r'data-name="([^"]+)"', block).group(1)
    url = 'https://lanpaper.local/' + name
    empty = 'shot__empty' in block
    has_image = 'shot__img' in block
    versions = re.search(r'menu__key">(\d+)<', block)

    # 1) hover-полоса быстрых действий больше не нужна
    if '<span class="link__quick">' in block:
        before, _quick, after = cut_span(block, '<span class="link__quick">')
        block = before + after

    # 2) старое ⋯-меню уезжает: его место теперь в футере
    if '<div class="menu-wrap">' in block:
        before, _old, after = cut_div(block, '<div class="menu-wrap">')
        block = before + after

    # подчищаем пустые строки внутри строки действий
    block = re.sub(r'<span class="link__actions">\s*</span>\s*', '', block, count=1)

    foot = FOOT_TPL.format(
        style='primary' if empty else 'secondary',
        icon=ic('pick' if has_image else 'upload', 1.9),
        label='Изменить' if has_image else 'Загрузить',
        caret=ic('caret', 2.2, size=11, cls='btn__caret'),
        menu_title='Заменить медиа' if has_image else 'Добавить медиа',
        add_items=ADD_MEDIA,
        title='/' + name,
        url=url,
        sliders=ic('sliders', 1.9),
        dots=ic('dots'),
        more_menu=more_items('/' + name, url, empty=empty,
                             versions=versions.group(1) if versions else ''),
    )

    idx = block.rstrip().rfind('</div>')
    return block[:idx] + foot + block[idx:]


def rebuild_cards(text, with_foot_only=False):
    parts = text.split('</article>')
    out = []
    for part in parts[:-1]:
        start = part.rfind('<article')
        is_card = start != -1 and 'data-item' in part[start:start + 400]
        done = 'link__foot' in part[start:]
        if is_card and not (with_foot_only and done):
            out.append(part[:start] + build_card(part[start:]) + '</article>')
        else:
            out.append(part + '</article>')
    return ''.join(out) + parts[-1]


s = rebuild_cards(s)
print('футеры добавлены:', len(card_names()), 'карточкам')

# ============================================================
# 4. Новые карточки под разные типы медиа
# ============================================================
FMT = {'jpg': 'JPEG', 'jpeg': 'JPEG', 'png': 'PNG', 'webp': 'WebP', 'gif': 'GIF',
       'mp4': 'MP4', 'webm': 'WebM', 'svg': 'SVG', 'ico': 'ICO'}
KNOWN_RATIO = {(16, 9): '16:9', (4, 3): '4:3', (3, 2): '3:2', (1, 1): '1:1',
               (9, 16): '9:16', (21, 9): '7:3', (4, 1): '4:1', (3, 4): '3:4'}


def gcd(a, b):
    while b:
        a, b = b, a % b
    return a


def ratio_of(dims):
    m = re.match(r'(\d+)\s*[×x]\s*(\d+)', dims or '')
    if not m:
        return None
    w, h = int(m.group(1)), int(m.group(2))
    g = gcd(w, h)
    return KNOWN_RATIO.get((w // g, h // g))


def badge(fmt, ratio=None):
    out = f'<span class="shot__fmt">{fmt}</span>'
    if ratio:
        out += f'\n            <span class="shot__fmt shot__fmt--ratio">{ratio}</span>'
    return out


NEW_CARDS = [dict(
    comment='/signage — вертикальный постер 9:16: показываем целиком, фон размыт',
    search='signage стойка вертикальный постер vertical-poster.jpg', tags='image local',
    name='signage', date='2026-10-02T10:15', size='8.4',
    shot_class=' shot--matte', img='media/wp-vertical.jpg', blur='media/wp-vertical.jpg',
    play='', badges=badge('JPEG', '9:16'),
    status='Публичный',
    file='vertical-poster.jpg', fmt='JPEG', dims='1080×1920', weight='8.4 МБ', views='47',
), dict(
    comment='/app-icon — PNG с прозрачностью: шахматка и запас по краям',
    search='app-icon иконка приложения png прозрачность логотип', tags='icon',
    name='app-icon', date='2026-10-03T12:05', size='0.08',
    shot_class=' shot--matte shot--alpha shot--icon', img='media/app-icon.png', blur='',
    play='', badges=badge('PNG', '512×512'),
    status='Публичный',
    file='app-icon.png', fmt='PNG · прозрачность', dims='512×512', weight='84 КБ', views='210',
), dict(
    comment='/panorama — панорама 21:9: кроп по рамке, подпись формата',
    search='panorama панорама city-panorama.jpg широкий', tags='image',
    name='panorama', date='2026-10-01T09:30', size='18.2',
    shot_class=' shot--pano', img='media/wp-panorama.jpg', blur='', play='',
    badges=badge('JPEG', '21:9'),
    status='Публичный',
    file='city-panorama.jpg', fmt='JPEG', dims='8000×2000', weight='18.2 МБ', views='88',
), dict(
    comment='/brand-tile — квадрат 1:1 (обои, плитка): матуем с размытым фоном',
    search='brand-tile плитка бренда webp квадрат обои', tags='icon',
    name='brand-tile', date='2026-09-30T15:40', size='0.4',
    shot_class=' shot--matte ', img='media/wp-square.jpg', blur='media/wp-square.jpg',
    play='', badges=badge('WebP', '1:1'),
    status='Публичный',
    file='brand-tile.webp', fmt='WebP', dims='1200×1200', weight='412 КБ', views='63',
), dict(
    comment='/animation — GIF: кроп по рамке, в подписи видно, что это анимация',
    search='animation анимация gif loop.gif луп', tags='image',
    name='animation', date='2026-09-29T18:20', size='2.1',
    shot_class='', img='media/wp-city.jpg', blur='', play='',
    badges=badge('GIF · анимация', '4:3'),
    status='Публичный',
    file='loop.gif', fmt='GIF', dims='800×600', weight='2.1 МБ', views='154',
), dict(
    comment='/menu-board — вертикальное видео 9:16: кадр целиком, длительность на месте',
    search='menu-board меню видео вертикальное board.mp4 стойка', tags='video local',
    name='menu-board', date='2026-10-03T07:45', size='24.6',
    shot_class=' shot--matte', img='media/wp-mono.jpg', blur='media/wp-mono.jpg',
    play='\n            <span class="shot__play"><span>' + ic('play') + '</span></span>'
         '\n            <span class="shot__dur">0:18</span>',
    badges=badge('MP4', '9:16'),
    status='Публичный',
    file='board.mp4', fmt='MP4', dims='1080×1920', weight='24.6 МБ', views='96',
)]

CARD_TPL = '''
        <!-- {comment} -->
        <article class="link" data-item data-search="{search}" data-tags="{tags}" data-name="{name}" data-date="{date}" data-size="{size}">
          <div class="link__select" data-select>
            <label class="check"><input type="checkbox" aria-label="Выбрать /{name}"><span class="check__box"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg></span></label>
          </div>
          <div class="shot{shot_class}">
            {blur_tag}<img class="shot__img" src="{img}" alt="" loading="lazy">{play}
            {badges}
            <span class="shot__bl"><span class="badge"><i style="width:6px;height:6px;border-radius:50%;background:#4fd39a;display:inline-block"></i>{status}</span></span>
          </div>
          <div class="link__body">
            <div class="link__row">
              <h3 class="link__name">/{name}</h3>
              <span class="link__actions">
                <button class="iconbtn iconbtn--sm" data-copy="https://lanpaper.local/{name}" aria-label="Копировать URL" title="Копировать URL">{copy_icon}</button>
              </span>
            </div>
            <p class="link__file">{file}</p>
            <p class="link__meta"><span>{fmt}</span><i>·</i><span>{dims}</span><i>·</i><span>{weight}</span><i>·</i><span class="link__views">{eye}{views}</span></p>
          </div>
        </article>
'''

blocks = []
for c in NEW_CARDS:
    blur_tag = (f'\n            <span class="shot__blur" style="background-image:url({c["blur"]})" '
                'aria-hidden="true"></span>') if c['blur'] else ''
    blocks.append(CARD_TPL.format(copy_icon=ic('copy', 1.9), eye=ic('eye', 1.9), blur_tag=blur_tag, **c))

marker = '\n\n    <!-- Пусто после фильтрации -->'
assert marker in s, 'маркер пустого состояния не найден'
s = s.replace(marker, ''.join(blocks) + marker, 1)

s = rebuild_cards(s, with_foot_only=True)
print('всего карточек:', len(card_names()), '| футеров:', s.count('link__foot--split'))

# ============================================================
# 5. Подписи формата и пропорций на существующих превью
# ============================================================
added = 0
for name in card_names():
    block = card_block(name)
    if 'shot__img' not in block or 'shot__fmt' in block:
        continue
    ext = re.search(r'link__file">[^<]*\.(' + '|'.join(FMT) + r')\b', block, re.I)
    if not ext:
        continue
    fmt = FMT[ext.group(1).lower()]
    dims = re.search(r'<span>(\d+×\d+)</span>', block)
    b = badge(fmt, ratio_of(dims.group(1)) if dims else None)
    block2 = block.replace('<img class="shot__img"', b + '\n            <img class="shot__img"', 1)
    if 'data-tags="video' in block and 'shot__dur' not in block:
        block2 = block2.replace('<span class="shot__play">',
                                '<span class="shot__dur">1:12</span>\n            <span class="shot__play">', 1)
    s = s.replace(block, block2, 1)
    added += 1

# плейлист: счётчик файлов и подпись «Плейлист · 6»
block = card_block('lobby')
s = s.replace(block, block.replace('Плейлист · 6</span></span>',
                                   'Плейлист</span></span>\n          <span class="shot__count">6 файлов</span>', 1), 1)
# пустая ссылка: пунктирная рамка вместо битой картинки
block = card_block('reception')
s = s.replace(block, block.replace('<div class="shot">', '<div class="shot shot--none">', 1), 1)
print('подписи расставлены:', added + 2)

# ============================================================
# 6. Фильтры и счётчики
# ============================================================
chips = '''<div class="chips filters__chips">
        <button class="chip" data-filter="all" data-filter-kind="single" aria-pressed="true">Все <span class="chip__count">13</span></button>
        <button class="chip" data-filter="pinned" aria-pressed="false">Закреплённые <span class="chip__count">0</span></button>
        <button class="chip" data-filter="playlist" aria-pressed="false">Плейлисты <span class="chip__count">0</span></button>
        <button class="chip" data-filter="video" aria-pressed="false">Видео <span class="chip__count">0</span></button>
        <button class="chip" data-filter="image" aria-pressed="false">Картинки <span class="chip__count">0</span></button>
        <button class="chip" data-filter="icon" aria-pressed="false">Иконки <span class="chip__count">0</span></button>
        <button class="chip" data-filter="local" aria-pressed="false">Только LAN <span class="chip__count">0</span></button>
      </div>'''
s = re.sub(r'<div class="chips filters__chips">.*?</div>', chips, s, count=1, flags=re.S)
s = s.replace('<b data-result-count>7</b> из 7', '<b data-result-count>13</b> из 13')
s = s.replace('<span>2.1 ГБ медиа</span>', '<span>2.4 ГБ медиа</span>')

# ============================================================
# 7. Пульт: подпись панели и клик по карточке
# ============================================================
sub('      <div class="drawer__title" data-drawer-title>/tv</div>',
    '''      <div class="drawer__eyebrow">Пульт ссылки</div>
      <div class="drawer__title" data-drawer-title>/tv</div>''')

sub('<script src="proto.js"></script>',
    '''<script src="proto.js"></script>
<script>
/* Палитра приложения: выбор запоминается в браузере */
(function () {
    var KEY = 'lp-palette';
    var root = document.documentElement;
    function setPalette(name, save) {
        root.setAttribute('data-palette', name);
        document.querySelectorAll('[data-palette-btn]').forEach(function (b) {
            b.setAttribute('aria-pressed', String(b.getAttribute('data-palette-btn') === name));
        });
        if (save) { try { localStorage.setItem(KEY, name); } catch (e) {} }
    }
    var saved = null;
    try { saved = localStorage.getItem(KEY); } catch (e) {}
    setPalette(saved || 'indigo', false);
    document.querySelectorAll('[data-palette-btn]').forEach(function (b) {
        b.addEventListener('click', function () { setPalette(b.getAttribute('data-palette-btn'), true); });
    });
})();

/* Клик по карточке открывает пульт ссылки.
   Кнопки, ссылки, чекбоксы и меню обрабатываются как обычно. */
(function () {
    var list = document.querySelector('[data-list]');
    document.querySelectorAll('[data-item]').forEach(function (card) {
        card.addEventListener('click', function (e) {
            if (e.target.closest('a, button, input, label, .menu-wrap')) return;
            if (list && list.classList.contains('is-selecting')) return;
            var name = card.getAttribute('data-name');
            if (!name || !window.protoDrawer) return;
            window.protoDrawer.open({ title: '/' + name, sub: 'https://lanpaper.local/' + name });
        });
    });
})();
</script>''')

s = re.sub(r'[ \t]+\n(\s*</(?:div|span|article|p)>)', r'\n\1', s)
s = re.sub(r'\n[ \t]*\n(\s*</div>\s*\n\s*<div class="link__body">)', r'\n\1', s)

open(BASE + 'final2.html', 'w', encoding='utf-8').write(s)
print('final2.html собран:', len(s.splitlines()), 'строк')
