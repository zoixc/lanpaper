// SPDX-License-Identifier: MIT
// Recompose the real panel, retaining IDs, nodes and event handlers.
(() => {
    document.documentElement.classList.add('radical');
    const $ = selector => document.querySelector(selector);
    const rail = document.createElement('aside');
    rail.className = 'catalog-rail';
    rail.setAttribute('aria-label', 'Навигация по коллекции');
    const label = document.createElement('p');
    label.className = 'catalog-label';
    label.textContent = 'ВАША БИБЛИОТЕКА';
    const bottom = document.createElement('div');
    bottom.className = 'catalog-signature';
    bottom.innerHTML = '<span class="catalog-symbol" aria-hidden="true">✳</span><span>Личное пространство.<br>Только ваши изображения.</span>';
    rail.append($('.brand'), $('#newLinkBtn'), label, $('.tools'), bottom);
    $('.shell').prepend(rail);
    $('.appbar__row--top').prepend($('.search'));
    $('.appbar__row--tools').remove();
    const eyebrow = document.createElement('div');
    eyebrow.className = 'catalog-eyebrow';
    eyebrow.innerHTML = '<span>LANPAPER / ЛИЧНЫЙ АРХИВ</span><span aria-hidden="true">01 — COLLECTION</span>';
    const intro = document.createElement('p');
    intro.className = 'catalog-intro';
    intro.textContent = 'Всё, что хочется сохранить перед глазами.';
    $('#library').prepend(eyebrow);
    $('.page-head').after(intro);
    // The default page and first prototype remain available in the lab bar.
    const brand = rail.querySelector('.brand');
    brand.href = '/admin?design=radical&sample=collection';
})();
