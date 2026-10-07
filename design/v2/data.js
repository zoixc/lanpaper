/* SPDX-License-Identifier: MIT */
/* ============================================================
   Демонстрационные данные стенда.
   Поля повторяют ответ /api/wallpapers один в один (плюс
   width/height/duration, которые фронт и так узнаёт из файла):
   благодаря этому перенос макета в приложение не требует
   переписывать разметку — меняется только источник данных.
   ============================================================ */
(function () {
    const MEDIA = '../prototypes/media/';
    const DAY = 86400;
    const now = Math.floor(Date.now() / 1000);

    const link = (o) => Object.assign({
        hasImage: true,
        imageUrl: '',
        preview: '',
        mimeType: 'image/jpeg',
        width: 0,
        height: 0,
        durationSec: 0,
        sizeBytes: 0,
        created: now - DAY,
        modTime: now - DAY,
        pinned: false,
        accessLevel: 'public',
        accessToken: '',
        currentVersion: 1,
        history: [],
        items: [],
        rotate: null,
        stats: null
    }, o);

    window.LP_DATA = [
        link({
            id: 'a1', linkName: 'tv', category: 'image',
            imageUrl: MEDIA + 'wp-dawn.jpg', preview: MEDIA + 'wp-dawn.jpg',
            width: 1376, height: 768, sizeBytes: 76_800,
            created: now - 46 * DAY, modTime: now - 9 * DAY,
            pinned: true, accessLevel: 'public', currentVersion: 3,
            history: [
                { version: 2, sizeBytes: 214_000, mtime: now - 21 * DAY, mimeType: 'image/jpeg' },
                { version: 1, sizeBytes: 198_400, mtime: now - 46 * DAY, mimeType: 'image/jpeg' }
            ],
            stats: { hits: 12480, bytes: 1_240_000_000, last: now - 240 }
        }),
        link({
            id: 'a2', linkName: 'lobby', category: 'image',
            imageUrl: MEDIA + 'wp-panorama.jpg', preview: MEDIA + 'wp-panorama.jpg',
            width: 1584, height: 672, sizeBytes: 92_160,
            created: now - 31 * DAY, modTime: now - 31 * DAY,
            accessLevel: 'local',
            stats: { hits: 304, bytes: 28_000_000, last: now - 3600 }
        }),
        link({
            id: 'a3', linkName: 'signage', category: 'image',
            imageUrl: MEDIA + 'wp-vertical.jpg', preview: MEDIA + 'wp-vertical.jpg',
            width: 768, height: 1376, sizeBytes: 69_632,
            created: now - 18 * DAY, modTime: now - 4 * DAY,
            currentVersion: 2,
            history: [{ version: 1, sizeBytes: 88_000, mtime: now - 18 * DAY, mimeType: 'image/jpeg' }],
            stats: { hits: 2210, bytes: 154_000_000, last: now - 900 }
        }),
        link({
            id: 'a4', linkName: 'menu-board', category: 'video',
            imageUrl: MEDIA + 'wp-square.jpg', preview: MEDIA + 'wp-square.jpg',
            mimeType: 'video/mp4', width: 1080, height: 1920, durationSec: 18,
            sizeBytes: 8_808_038,
            created: now - 12 * DAY, modTime: now - 2 * DAY,
            stats: { hits: 860, bytes: 7_400_000_000, last: now - 1200 }
        }),
        link({
            id: 'a5', linkName: 'app-icon', category: 'image',
            imageUrl: MEDIA + 'app-icon.png', preview: MEDIA + 'app-icon.png',
            mimeType: 'image/png', width: 512, height: 512, sizeBytes: 10_240,
            created: now - 26 * DAY, modTime: now - 26 * DAY,
            accessLevel: 'token', accessToken: 'k7f3q9v2',
            stats: { hits: 42, bytes: 430_000, last: now - 5 * 3600 }
        }),
        link({
            id: 'a6', linkName: 'brand-tile', category: 'image',
            imageUrl: MEDIA + 'wp-square.jpg', preview: MEDIA + 'wp-square.jpg',
            mimeType: 'image/webp', width: 1024, height: 1024, sizeBytes: 135_168,
            created: now - 24 * DAY, modTime: now - 24 * DAY,
            accessLevel: 'auth'
        }),
        link({
            id: 'a7', linkName: 'animation', category: 'gif',
            imageUrl: MEDIA + 'wp-abstract.jpg', preview: MEDIA + 'wp-abstract.jpg',
            mimeType: 'image/gif', width: 1376, height: 768, sizeBytes: 2_150_400,
            created: now - 15 * DAY, modTime: now - 15 * DAY,
            accessLevel: 'local',
            stats: { hits: 1290, bytes: 2_700_000_000, last: now - 7200 }
        }),
        link({
            id: 'a8', linkName: 'gallery', category: 'image',
            imageUrl: MEDIA + 'wp-city.jpg', preview: MEDIA + 'wp-city.jpg',
            width: 1376, height: 768, sizeBytes: 117_760,
            created: now - 40 * DAY, modTime: now - 3 * DAY,
            currentVersion: 2,
            history: [{ version: 1, sizeBytes: 96_000, mtime: now - 40 * DAY, mimeType: 'image/jpeg' }],
            items: [
                { id: '1', mimeType: 'image/jpeg', sizeBytes: 96_000, mtime: now - 33 * DAY },
                { id: '2', mimeType: 'image/jpeg', sizeBytes: 214_000, mtime: now - 27 * DAY },
                { id: '3', mimeType: 'image/webp', sizeBytes: 135_168, mtime: now - 20 * DAY },
                { id: '4', mimeType: 'video/mp4', sizeBytes: 4_200_000, mtime: now - 12 * DAY },
                { id: '5', mimeType: 'image/jpeg', sizeBytes: 88_000, mtime: now - 6 * DAY },
                { id: '6', mimeType: 'image/jpeg', sizeBytes: 92_160, mtime: now - 3 * DAY }
            ],
            rotate: { enabled: true, intervalSec: 30, order: 'sequential' },
            stats: { hits: 9820, bytes: 940_000_000, last: now - 60 }
        }),
        link({
            id: 'a9', linkName: 'forest', category: 'image',
            imageUrl: MEDIA + 'wp-forest.jpg', preview: MEDIA + 'wp-forest.jpg',
            width: 1376, height: 768, sizeBytes: 222_208,
            created: now - 8 * DAY, modTime: now - 8 * DAY,
            accessLevel: 'token', accessToken: 'r4m8x1pt',
            stats: { hits: 615, bytes: 137_000_000, last: now - 1800 }
        }),
        link({
            id: 'b1', linkName: 'mono', category: 'image',
            imageUrl: MEDIA + 'wp-mono.jpg', preview: MEDIA + 'wp-mono.jpg',
            width: 1376, height: 768, sizeBytes: 105_472,
            created: now - 5 * DAY, modTime: now - 5 * DAY,
            accessLevel: 'auth'
        }),
        link({
            id: 'b2', linkName: 'studio', category: 'empty',
            hasImage: false, imageUrl: '', preview: '',
            width: 0, height: 0, sizeBytes: 0,
            created: now - 2 * DAY, modTime: now - 2 * DAY
        }),
        link({
            id: 'b3', linkName: 'reception', category: 'image',
            imageUrl: MEDIA + 'wp-abstract.jpg', preview: MEDIA + 'wp-abstract.jpg',
            width: 1376, height: 768, sizeBytes: 65_536,
            created: now - 2 * DAY, modTime: now - 2 * DAY,
            accessLevel: 'local'
        })
    ];

    /* Конфигурация сервера — из тех же ручек, что читает админка. */
    window.LP_CONFIG = {
        historyLimit: 3,
        historyBudgetBytes: 536_870_912,
        compression: { quality: 82, scale: 100 },
        maxPixels: 36_000_000,
        schemes: ['public', 'local', 'token', 'auth'],
        langs: ['ru', 'en', 'de', 'fr', 'it', 'es']
    };
})();
