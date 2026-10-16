/* SPDX-License-Identifier: MIT */

const ACCESS = Object.freeze({
    public: Object.freeze({ icon: 'globe', key: 'access_public' }),
    local: Object.freeze({ icon: 'lock', key: 'access_local' }),
    token: Object.freeze({ icon: 'key', key: 'access_token' }),
    auth: Object.freeze({ icon: 'user', key: 'access_auth' })
});

export function accessMeta(level) {
    return ACCESS[level] || ACCESS.public;
}

export function entryMeta(entry, formatBytes) {
    return [
        entry.ext ? String(entry.ext).toUpperCase() : '',
        entry.sizeBytes ? formatBytes(entry.sizeBytes) : ''
    ].filter(Boolean).join(' · ');
}

export function entryTitle(entry, formatBytes) {
    return entryMeta(entry, formatBytes) || formatBytes(entry.sizeBytes);
}

/* ============================================================
   МЕДИА ПО ССЫЛКЕ
   Панель ничего не скачивает сама: адрес получает сервер, и он же решает,
   медиа там или нет. Проверка на этой стороне только повторяет то, что
   сервер отвергнет заведомо (схема, длина), и подсказывает, чего ждать.
   Расширение не проверяется вовсе: у картинок на CDN его может не быть
   (images.unsplash.com отдаёт файл по /photo-…?w=…), а тип сервер
   определяет по содержимому, а не по имени.
   ============================================================ */

/* Тот же предел, что у сервера (handlers/upload.go): «URL too long». Считается
   в байтах UTF-8, ровно как len() в Go: у адреса с кириллическим путём
   символов меньше, чем байтов, и панель не должна отправлять то, что сервер
   отвергнет по длине. */
export const MEDIA_URL_MAX = 2048;

const utf8Length = value => new TextEncoder().encode(value).length;

/* Расширения из config.AllowedMediaExts — только для подсказки. Список и
   список сервера обязаны совпадать: панель не должна обещать приём файла,
   который сервер отвергнет, и наоборот. */
const MEDIA_EXT = /\.(jpe?g|png|gif|webp|avif|bmp|tiff?|mp4|webm)\/?$/i;
/* Любое другое расширение — тоже не запрет: `image.php?id=3` отдаёт
   картинку, а ссылка на страницу — нет, и решает это содержимое ответа. */
const OTHER_EXT = /\.[a-z0-9]{1,8}\/?$/i;

/* Адрес ровно в том виде, в каком его поймёт сервер. Всё, что здесь
   снимается, — артефакты копирования, а не часть ссылки:
   - пробелы по краям и обёртка <…> (так ссылку вставляют Slack и почта);
   - кавычки в паре («https://…» — так адрес выглядит в чате и в JSON);
   - markdown-ссылка [подпись](адрес);
   - закрывающая скобка без пары (адрес вынут из текста «(см. https://…)»);
   - невидимые символы: нулевой пробел, мягкий перенос, BOM, метки
     направления. В ссылку они попадают из PDF, Word и мессенджеров и ломают
     разбор адреса на сервере;
   - «&amp;» вместо «&» — так выглядят ссылки из исходника страницы,
     markdown-файла и лога чата. Это не косметика: у imgix (а на нём работает
     Unsplash) параметры после «&amp;» доходят как «amp;w=1336», неизвестные
     параметры он игнорирует, и вместо уменьшенной картинки отдаёт
     полноразмерный оригинал. */
export function normalizeMediaUrl(raw) {
    let value = String(raw == null ? '' : raw).trim();
    const markdown = /^\[[^\]]*\]\(\s*([^)\s]+)\s*\)$/.exec(value);
    if (markdown) value = markdown[1].trim();
    if (value.length > 1 && value.startsWith('<') && value.endsWith('>')) {
        value = value.slice(1, -1).trim();
    }
    if (value.length > 1) {
        const first = value[0];
        const last = value[value.length - 1];
        if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
            value = value.slice(1, -1).trim();
        }
    }
    value = value.replace(/[\u200b-\u200f\u2060\ufeff\u00ad]/g, '');
    /* Скобки вокруг адреса снимаются, только если внутри действительно адрес:
       у ссылки, где скобка — часть пути (Wikipedia), ничего не меняется.
       Непарная закрывающая скобка — след текста («см. (https://…)»), её
       снимаем отдельно и только пока она непарная. */
    if ((value.startsWith('(') && value.endsWith(')')) || (value.startsWith('[') && value.endsWith(']'))) {
        const inner = value.slice(1, -1).trim();
        if (/^https?:\/\//i.test(inner)) value = inner;
    }
    while (value.endsWith(')') && value.split(')').length > value.split('(').length) {
        value = value.slice(0, -1);
    }
    return asciiHostAddress(value.replace(/&amp;/gi, '&'));
}

/* Хост нелатинского адреса умеет приводить к ASCII только разборщик URL:
   «пример.рф» для него — «xn--e1afmkfd.xn--p1ai». Сервер получает строку и
   отдаёт её в DNS как есть, кириллическое имя там не находится, и рабочий
   адрес кончается ответом «не удалось связаться с адресом». Приводим хост
   здесь, а всё остальное в строке оставляем буква в букву: путь, запрос и
   даже отсутствие «/» — ровно тот адрес, который уйдёт в запрос. */
function asciiHostAddress(value) {
    let url;
    try {
        url = new URL(value);
    } catch (_) {
        return value; // не адрес — пусть сервер скажет, что с ним не так
    }
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return value;
    /* Адрес с логином и паролем сервер отвергает целиком («Address is not
       allowed»): подменить его на другой значило бы скачать не то, что
       просили, поэтому такой адрес остаётся как есть. */
    if (url.username || url.password) return value;
    const start = value.indexOf('//') + 2;
    if (start < 2) return value;
    let end = value.length;
    for (let i = start; i < value.length; i++) {
        const ch = value[i];
        if (ch === '/' || ch === '?' || ch === '#') { end = i; break; }
    }
    if (!/[^\x00-\x7F]/.test(value.slice(start, end))) return value;
    return value.slice(0, start) + url.host + value.slice(end);
}

/* Что панель может сказать об адресе до отправки. Путь отделяется от
   параметров: «?v=2.0» — это не расширение файла, а версия в запросе. */
export function mediaUrlKind(value) {
    const url = String(value == null ? '' : value);
    if (!url) return 'empty';
    if (utf8Length(url) > MEDIA_URL_MAX) return 'too-long';
    if (!/^https?:\/\//i.test(url)) return 'invalid';
    /* Схема есть, значит дальше адрес должен быть настоящим: пробел (в том
       числе неразрывный, который приходит из PDF), «%» не перед двумя
       шестнадцатеричными цифрами — это обрывок ссылки, а не ссылка. Сервер
       разберёт её так же (url.Parse в Go), но объяснить лучше здесь. */
    if (/\s/.test(url) || /%(?![0-9a-f]{2})/i.test(url)) return 'malformed';
    try {
        new URL(url);
    } catch (_) {
        return 'malformed';
    }
    const path = url.split('#')[0].split('?')[0];
    if (MEDIA_EXT.test(path)) return 'media';
    if (OTHER_EXT.test(path)) return 'other-extension';
    return 'no-extension';
}

export function uploadForm(linkName, mode, sourceName, source) {
    const form = new FormData();
    form.append('linkName', linkName);
    if (mode === 'append') form.append('mode', 'append');
    form.append(sourceName, source);
    return form;
}

export function panelSnapshot(state) {
    return Object.freeze({
        lang: state.lang,
        theme: state.theme,
        view: state.view,
        sort: state.sort,
        links: state.links.map(link => Object.assign({}, link))
    });
}
