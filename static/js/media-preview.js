/* SPDX-License-Identifier: MIT */

// No src and no video element exist until an explicit user gesture. The caller
// supplies the authenticated same-origin admin route, never an access token URL.
export function createVideoPreview(doc, options) {
    const box = doc.createElement('div');
    box.className = 'video-preview';
    const button = doc.createElement('button');
    button.type = 'button';
    button.className = 'btn btn--soft video-preview__start';
    button.textContent = options.label;
    box.append(button);
    button.addEventListener('click', () => {
        if (!box.contains(button)) return;
        const video = doc.createElement('video');
        video.className = 'video-preview__player';
        video.controls = true;
        video.playsInline = true;
        video.preload = 'none';
        video.setAttribute('aria-label', options.label);
        function showError() {
            if (!box.contains(video) || !video.hasAttribute('src')) return;
            video.removeEventListener('error', showError);
            stopMedia(box);
            const message = doc.createElement('p');
            message.className = 'field-hint field-hint--error';
            message.setAttribute('role', 'status');
            message.textContent = options.error;
            box.replaceChildren(message);
        }
        video.addEventListener('error', showError, { once: true });
        box.replaceChildren(video);
        video.src = options.src;
        // play() is called only in the user activation handler. Rejection may
        // simply mean the browser wants its native play control; keep it usable.
        const playing = video.play();
        if (playing && playing.catch) playing.catch(error => {
            // Some engines reject unsupported playback before firing 'error'.
            // Permission rejection and deliberate load cancellation are not
            // corrupt media: keep native controls usable in those cases.
            if (error.name === 'NotSupportedError' || video.error) showError();
        });
    });
    return box;
}

export function stopMedia(root) {
    root.querySelectorAll('video').forEach(video => {
        video.pause();
        video.removeAttribute('src');
        video.load();
    });
}
