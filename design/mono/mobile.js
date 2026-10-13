// SPDX-License-Identifier: MIT
const phone = document.getElementById('phone');
document.querySelectorAll('[data-width]').forEach(button => {
    button.addEventListener('click', () => {
        // Eight-pixel frame on each side: the iframe gets the stated width.
        phone.style.width = `${Number(button.dataset.width) + 16}px`;
        document.querySelectorAll('[data-width]').forEach(item => {
            item.setAttribute('aria-pressed', String(item === button));
        });
    });
});
